package main

import "sync"

var (
	configsMu sync.Mutex
	configs   = map[int]*pluginConfig{}
	nextID    int
)

func storeConfig(conf *pluginConfig) int {
	configsMu.Lock()
	defer configsMu.Unlock()

	id := nextID
	nextID++
	configs[id] = conf
	return id
}

func getConfig(id int) *pluginConfig {
	configsMu.Lock()
	defer configsMu.Unlock()
	return configs[id]
}

func takeConfig(id int) *pluginConfig {
	configsMu.Lock()
	defer configsMu.Unlock()

	conf := configs[id]
	delete(configs, id)
	return conf
}
