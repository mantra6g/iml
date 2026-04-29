package cmp

func ElementsMatchInAnyOrder[T comparable](p1, p2 []T) bool {
	if len(p1) != len(p2) {
		return false
	}
	counts := make(map[T]int)
	for _, v := range p1 {
		counts[v]++
	}
	for _, v := range p2 {
		counts[v]--
		if counts[v] < 0 {
			return false
		}
	}
	return true
}
