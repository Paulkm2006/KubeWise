package casefile

import "sort"

type Catalog struct {
	byID map[string]Evidence
}

func NewCatalog() *Catalog {
	return &Catalog{byID: make(map[string]Evidence)}
}

func (c *Catalog) Add(evs ...Evidence) {
	if c == nil {
		return
	}
	for _, ev := range evs {
		if ev.ID == "" {
			continue
		}
		c.byID[ev.ID] = ev
	}
}

func (c *Catalog) All() []Evidence {
	if c == nil {
		return nil
	}
	ids := make([]string, 0, len(c.byID))
	for id := range c.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Evidence, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.byID[id])
	}
	return out
}

func (c *Catalog) Exists(id string) bool {
	if c == nil {
		return false
	}
	_, ok := c.byID[id]
	return ok
}
