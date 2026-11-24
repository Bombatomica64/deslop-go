package sched

import "fx/queue"

func Admit(q *queue.RunQueue, x int) { q.Items = append(q.Items, x) }
func Peek(q *queue.RunQueue) int     { return q.Head + q.HeadValue() + q.Limit }
func Fill(q *queue.RunQueue)         { q.SpareRef()["a"] = 1 }
func Count(q *queue.RunQueue) int    { m := q.SpareRef(); return len(m) }
func Use(r *queue.Row)               { r.A[0] = 1 }
