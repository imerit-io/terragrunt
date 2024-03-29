package services

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/models"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/terraform/command/cliconfig"
	"golang.org/x/sync/errgroup"
)

var (
	unzipFileMode      = os.FileMode(0000)
	retryDelayLockfile = time.Second * 5
	maxRetriesLockfile = 60 // equals 5 mins
)

// Borrow the "unpack a zip cache into a target directory" logic from
// go-getter
var unzip = getter.ZipDecompressor{}

// Borrow the "unpack a zip cache into a target directory" logic from go-getter
type ProviderCaches []*ProviderCache

func (providers ProviderCaches) Find(target *models.Provider) *ProviderCache {
	for _, provider := range providers {
		if provider.Match(target) {
			return provider
		}
	}

	return nil
}

type ProviderCache struct {
	*ProviderService
	*models.Provider

	needCacheArchive bool
	ready            bool
}

func (cache *ProviderCache) providerDir() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path())
}

func (cache *ProviderCache) lockFilename() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), cache.Platform()) + ".lock"
}

func (cache *ProviderCache) platformDir() string {
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) terraformPluginPlatformDir() string {
	return filepath.Join(cache.terraformPluginDir, cache.Provider.Path(), cache.Platform())
}

func (cache *ProviderCache) ArchiveFilename() string {
	if cache.DownloadURL == nil {
		return ""
	}
	return filepath.Join(cache.baseCacheDir, cache.Provider.Path(), filepath.Base(cache.DownloadURL.Path))
}

// warmUp checks if the binary file already exists in the cache directory, if not, downloads the archive and unzip it.
func (cache *ProviderCache) warmUp(ctx context.Context) error {
	var (
		terraformPluginPlatformDir = cache.terraformPluginPlatformDir()
		platformDir                = cache.platformDir()
		providerDir                = cache.providerDir()
		archiveFilename            = cache.ArchiveFilename()
		lockfileName               = cache.lockFilename()

		alreadyCached bool
	)

	if err := os.MkdirAll(providerDir, os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

	lockfile, err := util.AcquireLockfile(ctx, lockfileName, maxRetriesLockfile, retryDelayLockfile)
	if err != nil {
		return err
	}
	defer lockfile.Unlock()

	if !util.FileExists(platformDir) {
		if util.FileExists(terraformPluginPlatformDir) {
			log.Debugf("Create symlink file %s to %s", platformDir, terraformPluginPlatformDir)
			if err := os.Symlink(terraformPluginPlatformDir, platformDir); err != nil {
				return errors.WithStackTrace(err)
			}
			alreadyCached = true
		} else {
			if err := os.MkdirAll(platformDir, os.ModePerm); err != nil {
				return errors.WithStackTrace(err)
			}
			cache.needCacheArchive = true
		}
	} else {
		alreadyCached = true
	}

	if cache.needCacheArchive && !util.FileExists(archiveFilename) {
		if cache.DownloadURL == nil {
			return errors.Errorf("failed to cache provider %q, the download URL is undefined", cache.Provider)
		}

		log.Debugf("Fetching provider %s", cache.Provider)
		if err := util.FetchFile(ctx, cache.DownloadURL.String(), archiveFilename); err != nil {
			return err
		}
	}

	if !alreadyCached {
		log.Debugf("Decompress file %s", archiveFilename)
		if err := unzip.Decompress(platformDir, archiveFilename, true, unzipFileMode); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

func (cache *ProviderCache) removeArchive(ctx context.Context) error {
	var (
		archiveFilename = cache.ArchiveFilename()
	)

	if cache.needCacheArchive && util.FileExists(archiveFilename) {
		log.Debugf("Remove archive file %s", archiveFilename)
		if err := os.Remove(archiveFilename); err != nil {
			return errors.WithStackTrace(err)
		}
	}

	return nil
}

type ProviderService struct {
	baseCacheDir string

	providerCaches  ProviderCaches
	providerCacheCh chan *ProviderCache

	cacheMu      sync.RWMutex
	cacheReadyMu sync.RWMutex

	// If needCacheArchive is true, ensures that not only the unarchived binary is cached, but also its archive. We need acrhives in order to reduce the bandwidth, because `terraform lock provider` always loads providers from a remote registry to create a lock file rather than using a cached one. This is only used when opts.ProviderCompleteLock is true.
	needCacheArchive   bool
	terraformPluginDir string
}

func NewProviderService(baseCacheDir string, needCacheArchive bool) *ProviderService {
	return &ProviderService{
		baseCacheDir:     baseCacheDir,
		providerCacheCh:  make(chan *ProviderCache),
		needCacheArchive: needCacheArchive,
	}
}

// WaitForCacheReady blocks the call until all providers are cached.
func (service *ProviderService) WaitForCacheReady() {
	service.cacheReadyMu.Lock()
	defer service.cacheReadyMu.Unlock()
}

// CacheProvider starts caching the given provider using non-blocking approch.
func (service *ProviderService) CacheProvider(provider *models.Provider) {
	service.cacheMu.Lock()
	defer service.cacheMu.Unlock()

	if cache := service.providerCaches.Find(provider); cache != nil {
		return
	}

	cache := &ProviderCache{
		ProviderService:  service,
		Provider:         provider,
		needCacheArchive: service.needCacheArchive,
	}

	service.providerCacheCh <- cache
	service.providerCaches = append(service.providerCaches, cache)
}

// GetProviderCache returns the requested provider archive cache, if it exists.
func (service *ProviderService) GetProviderCache(provider *models.Provider) *ProviderCache {
	service.cacheMu.RLock()
	defer service.cacheMu.RUnlock()

	if cache := service.providerCaches.Find(provider); cache != nil && cache.ready && util.FileExists(cache.ArchiveFilename()) {
		return cache
	}
	return nil
}

// RunCacheWorker is responsible to handle a new caching request and removing temporary files upon completion.
func (service *ProviderService) RunCacheWorker(ctx context.Context) error {
	if service.baseCacheDir == "" {
		return errors.Errorf("provider cache directory not specified")
	}
	log.Debugf("Provider cache dir %q", service.baseCacheDir)

	configDir, err := cliconfig.ConfigDir()
	if err != nil {
		return errors.WithStackTrace(err)
	}
	service.terraformPluginDir = filepath.Join(configDir, "plugins")

	errGroup, ctx := errgroup.WithContext(ctx)
	for {
		select {
		case cache, ok := <-service.providerCacheCh:
			if !ok {
				return nil
			}

			errGroup.Go(func() error {
				service.cacheReadyMu.RLock()
				defer service.cacheReadyMu.RUnlock()

				if err := cache.warmUp(ctx); err != nil {
					os.Remove(cache.platformDir())
					return err
				}
				cache.ready = true

				log.Infof("Provider %q is cached", cache.Provider)

				return nil
			})
		case <-ctx.Done():
			close(service.providerCacheCh)

			merr := &multierror.Error{}

			if err := errGroup.Wait(); err != nil {
				merr = multierror.Append(merr, err)
			}

			for _, cache := range service.providerCaches {
				if err := cache.removeArchive(ctx); err != nil {
					merr = multierror.Append(merr, errors.WithStackTrace(err))
				}
			}

			return merr.ErrorOrNil()
		}
	}
}
