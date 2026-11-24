package user

import "fx/box"

func Use(b *box.Box) int {
	n := 0
	for _, s := range b.Names {
		n += len(s)
	}
	_ = b.Now()
	return n + b.Size + len(b.Ids) + b.At.Day() + b.M["k"] + box.Box{Size: 1}.Size
}
