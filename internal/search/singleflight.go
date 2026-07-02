package search

import "sync"

// singleflightGroup es una implementación minimalista del patrón
// singleflight: llamadas concurrentes para la misma key comparten una sola
// ejecución y resultado. Esto evita que una ráfaga de usuarios buscando lo
// mismo dispare varias llamadas reales a SerpAPI/OpenRouter en paralelo.
type singleflightGroup struct {
	mu    sync.Mutex
	calls map[string]*call
}

type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

func newSingleflightGroup() *singleflightGroup {
	return &singleflightGroup{calls: make(map[string]*call)}
}

func (g *singleflightGroup) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}

	c := new(call)
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()

	return c.val, c.err
}
