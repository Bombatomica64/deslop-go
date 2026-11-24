package app

import (
	"fx/storea"
	"fx/storeb"
	"fx/storec"
)

func Run() {
	a := &storea.Store{}
	b := &storeb.Store{}
	c := &storec.Store{}
	a.M0()
	b.M0()
	a.M1()
	b.M1()
	a.M2()
	b.M2()
	a.M3()
	b.M3()
	a.M4()
	b.M4()
	a.M5()
	b.M5()
	a.M6()
	b.M6()
	a.M7()
	b.M7()
	c.M0()
	c.M1()
	c.M2()
	c.M3()
	c.M4()
	c.M5()
	c.M6()
}
