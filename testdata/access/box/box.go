package box

import "time"

type Box struct {
	Now   func() time.Time // (b.Now)() reads it: no leak
	Names []string         // range over plain data: no leak
	Size  int              // scalar: no leak
	Ids   []int            // len(): a call on a non-scalar: leak
	At    time.Time        // a method on it: leak
	M     map[string]int   // index: leak
	kept  int
}

func (b *Box) Work() int {
	b.kept++
	return len(b.Names) + b.Size + len(b.Ids) + b.M["x"] + b.At.Second() + b.Now().Second()
}
