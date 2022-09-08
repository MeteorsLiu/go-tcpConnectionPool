package connpool

import (
	"context"
	"net"
	"time"
)

type Opts []interface{}

func DefaultOpts() Opts {
	return Opts{}
}

func (o Opts) WithDialer(d *net.Dialer) {
	o = append(o, d)
}

func (o Opts) WithTimeout(t time.Duration) {
	o = append(o, t)
}

func (o Opts) WithMinSize(m int32) {
	o = append(o, m)
}

func (o Opts) WithContext(c context.Context) {
	o = append(o, c)
}

func (o Opts) Parse() (*net.Dialer, time.Duration, int32, context.Context) {
	var d *net.Dialer
	var t time.Duration
	var m int32
	var c context.Context
	for _, v := range o {
		switch typ := v.(type) {
		case int32:
			m = typ
		case time.Duration:
			t = typ
		case *net.Dialer:
			d = typ
		case context.Context:
			c = typ
		}
	}
	return d, t, m, c
}
