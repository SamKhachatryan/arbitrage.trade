package orderbook

import (
	"sync"
)

// GlobalManager manages all pair managers
type GlobalManager struct {
	mu           sync.RWMutex
	pairManagers map[string]*PairManager
	signalURL    string
	analyzer     *Analyzer
}

// NewGlobalManager creates a new global orderbook manager
func NewGlobalManager(signalURL string) *GlobalManager {
	return &GlobalManager{
		pairManagers: make(map[string]*PairManager),
		signalURL:    signalURL,
	}
}

// SetAnalyzer sets the analyzer for all current and future pair managers
func (gm *GlobalManager) SetAnalyzer(analyzer *Analyzer) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	gm.analyzer = analyzer

	// Set analyzer on all existing pair managers
	for _, pm := range gm.pairManagers {
		pm.SetAnalyzer(analyzer)
	}
}

// AddPair adds a new trading pair to monitor
func (gm *GlobalManager) AddPair(pairName string) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if _, exists := gm.pairManagers[pairName]; exists {
		return nil // Already monitoring
	}

	pm := NewPairManager(pairName, gm.signalURL)

	// Set analyzer if one exists
	if gm.analyzer != nil {
		pm.SetAnalyzer(gm.analyzer)
	}

	if err := pm.Start(); err != nil {
		return err
	}

	gm.pairManagers[pairName] = pm
	return nil
}

// RemovePair stops monitoring a trading pair
func (gm *GlobalManager) RemovePair(pairName string) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	if pm, exists := gm.pairManagers[pairName]; exists {
		pm.Stop()
		delete(gm.pairManagers, pairName)
	}
}

// GetPairManager returns the manager for a specific pair
func (gm *GlobalManager) GetPairManager(pairName string) (*PairManager, bool) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	pm, exists := gm.pairManagers[pairName]
	return pm, exists
}

// GetAllPairs returns all monitored pair names
func (gm *GlobalManager) GetAllPairs() []string {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	pairs := make([]string, 0, len(gm.pairManagers))
	for pairName := range gm.pairManagers {
		pairs = append(pairs, pairName)
	}
	return pairs
}

// StopAll stops all pair managers
func (gm *GlobalManager) StopAll() {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	for _, pm := range gm.pairManagers {
		pm.Stop()
	}
	gm.pairManagers = make(map[string]*PairManager)
}
