package okchild

import "fx/okchild/inner"

func A() int { return inner.B() }
