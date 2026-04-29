package routecalc

//
//import "github.com/google/uuid"
//
//type item struct {
//	node     uuid.UUID
//	dist     int
//	catIndex int // Index of the category in the categoryIDs slice
//}
//
//type priorityQueue []*item
//
//func (pq priorityQueue) Len() int           { return len(pq) }
//func (pq priorityQueue) Less(i, j int) bool { return pq[i].dist < pq[j].dist }
//func (pq priorityQueue) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }
//func (pq *priorityQueue) Push(x any) {
//	*pq = append(*pq, x.(*item))
//}
//func (pq *priorityQueue) Pop() any {
//	n := len(*pq)
//	it := (*pq)[n-1]
//	*pq = (*pq)[:n-1]
//	return it
//}
