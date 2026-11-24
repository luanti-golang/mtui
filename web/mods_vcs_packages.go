package web

import (
	"fmt"
	"io"
	"mtui/types"
	"net/http"
	"sync"
	"time"
)

const (
	vcsPackagesURL = "https://content.luanti.org/api/packages/?fmt=vcs"
	vcsPackagesTTL = 24 * time.Hour
)

var vcsPackagesClient = &http.Client{Timeout: 30 * time.Second}

// in-memory cache of the contentdb vcs package list
var vcsPackagesCache struct {
	mu          sync.Mutex
	data        []byte
	contentType string
	fetched     time.Time
}

func fetchVcsPackages() ([]byte, string, error) {
	vcsPackagesCache.mu.Lock()
	defer vcsPackagesCache.mu.Unlock()

	if vcsPackagesCache.data != nil && time.Since(vcsPackagesCache.fetched) < vcsPackagesTTL {
		return vcsPackagesCache.data, vcsPackagesCache.contentType, nil
	}

	resp, err := vcsPackagesClient.Get(vcsPackagesURL)
	if err != nil {
		return nil, "", fmt.Errorf("fetch vcs packages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch vcs packages: unexpected status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read vcs packages: %w", err)
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}

	vcsPackagesCache.data = data
	vcsPackagesCache.contentType = ct
	vcsPackagesCache.fetched = time.Now()
	return data, ct, nil
}

func (a *Api) GetVcsPackages(w http.ResponseWriter, r *http.Request, claims *types.Claims) {
	data, ct, err := fetchVcsPackages()
	if err != nil {
		SendError(w, http.StatusBadGateway, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Write(data)
}
