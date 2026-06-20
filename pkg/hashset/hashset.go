package hashset

type Set[T comparable] map[T]struct{}

func New[T comparable](is ...T) Set[T] {
	if len(is) == 0 {
		return make(Set[T])
	}
	s := make(Set[T], len(is))
	for _, i := range is {
		s.Add(i)
	}
	return s
}

func (s Set[T]) Add(i T) {
	s[i] = struct{}{}
}

func (s Set[T]) Remove(i T) {
	delete(s, i)
}

func (s Set[T]) Contains(i T) bool {
	_, ok := s[i]
	return ok
}

func (s Set[T]) Size() int {
	return len(s)
}
