package queue

type RunQueue struct {
	Items []int
	Head  int
	Limit int
	Spare map[string]int
	n     int
}

func (q *RunQueue) Full() bool               { return len(q.Items)+q.Head >= q.Limit && q.Spare != nil && q.n > 0 }
func (q *RunQueue) SpareRef() map[string]int { return q.Spare }
func (q *RunQueue) HeadValue() int           { return q.Head }
func (q *RunQueue) Push(x int)               { q.inner(x) }
func (q *RunQueue) inner(x int)              { q.Items = append(q.Items, x) }

// All public, no method: guards nothing.
type Row struct{ A, B []int }
