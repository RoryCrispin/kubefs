package resources

import "time"


type stateEntry struct {
	value any
	expiry *time.Time
}

type State struct  {
	store map[string]stateEntry
}

func NewState() *State {
	rv := State{}
	rv.store = make(map[string]stateEntry)
	return &rv
}

func (s *State) PutTTL(key string, value any, ttl time.Duration) {
	e := time.Now().Add(ttl)
	expiry := &e

	s.store[key] = stateEntry{
		value: value,
		expiry: expiry,
	}
}

func (s *State) Put(key string, value any) {
	s.store[key] = stateEntry{
		value: value,
	}
}

func (s *State) Get(key string) (any, bool) {
	v, exists := s.store[key]
	if !exists {
		return nil, false
	}
	if v.expiry != nil{
		if v.expiry.Before(time.Now()) {
			delete(s.store, key)
			return nil, false
		}
	}
	return v.value, true
}
