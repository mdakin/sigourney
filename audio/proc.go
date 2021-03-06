/*
Copyright 2013 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package audio

import (
	"math"
	"math/rand"

	"github.com/nf/sigourney/fast"
)

func sampleToHz(s Sample) float64 {
	return 440 * fast.Exp2(float64(s)*10)
}

func NewSquare() *Square {
	o := &Square{}
	o.inputs("pitch", &o.pitch, "syn", &o.syn)
	return o
}

type Square struct {
	sink
	pitch Processor // 0.1/oct, 0 == 440Hz
	syn   trigger

	pos float64
}

func (o *Square) Process(s []Sample) {
	o.pitch.Process(s)
	t := o.syn.Process()
	p := o.pos
	hz, lastS := sampleToHz(s[0]), s[0]
	for i := range s {
		if o.syn.isTrigger(t[i]) {
			p = 0
		}
		if s[i] != lastS {
			hz = sampleToHz(s[i])
		}
		p += hz
		if p > waveHz {
			p -= waveHz
		}
		if p > waveHz/2 {
			s[i] = -1
		} else {
			s[i] = 1
		}
	}
	o.pos = p
}

func NewSin() *Sin {
	o := &Sin{}
	o.inputs("pitch", &o.pitch, "syn", &o.syn)
	return o
}

type Sin struct {
	sink
	pitch Processor // 0.1/oct, 0 == 440Hz
	syn   trigger

	pos float64
}

func (o *Sin) Process(s []Sample) {
	o.pitch.Process(s)
	t := o.syn.Process()
	p := o.pos
	hz, lastS := sampleToHz(s[0]), s[0]
	for i := range s {
		if o.syn.isTrigger(t[i]) {
			p = 0
		}
		if s[i] != lastS {
			hz = sampleToHz(s[i])
		}
		s[i] = Sample(fast.Sin(p * 2 * math.Pi))
		p += hz / waveHz
		if p > 100 {
			p -= 100
		}
	}
	o.pos = p
}

func NewMul() *Mul {
	a := &Mul{}
	a.inputs("a", &a.a, "b", &a.b)
	return a
}

type Mul struct {
	sink
	a Processor
	b source
}

func (a *Mul) Process(s []Sample) {
	a.a.Process(s)
	m := a.b.Process()
	for i := range s {
		s[i] *= m[i]
	}
}

func NewSum() *Sum {
	s := &Sum{}
	s.inputs("a", &s.a, "b", &s.b)
	return s
}

type Sum struct {
	sink
	a Processor
	b source
}

func (s *Sum) Process(buf []Sample) {
	s.a.Process(buf)
	b := s.b.Process()
	for i := range buf {
		buf[i] += b[i]
	}
}

func NewEnv() *Env {
	e := &Env{}
	e.inputs("gate", &e.gate, "trig", &e.trig, "att", &e.att, "dec", &e.dec)
	return e
}

type Env struct {
	sink
	gate     Processor
	trig     trigger
	att, dec source

	v      Sample
	up     bool
	wasLow bool
}

func (e *Env) Process(s []Sample) {
	e.gate.Process(s)
	att, dec, t := e.att.Process(), e.dec.Process(), e.trig.Process()
	v := e.v
	for i := range s {
		trigger := e.trig.isTrigger(t[i])
		if !e.up {
			if trigger || v < s[i] {
				e.up = true
			} else if v > s[i] {
				if d := dec[i]; d > 0 {
					v -= 1 / (d * waveHz * 10)
				}
			}
		}
		if e.up {
			if a := att[i]; a > 0 {
				v += 1 / (a * waveHz * 10)
			}
		}
		if v > 1 {
			v = 1
			e.up = false
		} else if v < 0 {
			v = 0
		}
		s[i] = v
	}
	e.v = v
}

func NewClip() *Clip {
	c := &Clip{}
	c.inputs("in", &c.in)
	return c
}

type Clip struct {
	sink
	in Processor
}

func (c *Clip) Process(s []Sample) {
	c.in.Process(s)
	for i, v := range s {
		if v > 1 {
			s[i] = 1
		} else if v < -1 {
			s[i] = -1
		}
	}
}

type Value Sample

func (v Value) Process(s []Sample) {
	for i := range s {
		s[i] = Sample(v)
	}
}

func NewRand() *Rand {
	r := &Rand{}
	r.inputs("min", &r.min, "max", &r.max, "trig", &r.trig)
	return r
}

type Rand struct {
	sink
	min  Processor
	max  source
	trig trigger

	last Sample
}

func (r *Rand) Process(s []Sample) {
	r.min.Process(s)
	max, t := r.max.Process(), r.trig.Process()
	v := r.last
	for i := range s {
		if r.trig.isTrigger(t[i]) {
			v = s[i] + Sample(rand.Float64())*(max[i]-s[i])
		}
		s[i] = v
	}
	r.last = v
}

func NewDup(src Processor) *Dup {
	d := &Dup{src: src}
	return d
}

type Dup struct {
	src  Processor
	outs []*Output
	buf  []Sample
	done bool
}

func (d *Dup) Tick() {
	d.done = false
}

func (d *Dup) SetSource(p Processor) {
	d.src = p
}

func (d *Dup) Output() *Output {
	o := &Output{d: d}
	d.outs = append(d.outs, o)
	if len(d.outs) > 1 && d.buf == nil {
		d.buf = make([]Sample, nSamples)
	}
	return o
}

type Output struct {
	d *Dup
}

func (o *Output) Process(p []Sample) {
	if len(o.d.outs) == 1 {
		if !o.d.done {
			o.d.done = true
			o.d.src.Process(p)
		}
		return
	}
	if !o.d.done {
		o.d.done = true
		o.d.src.Process(o.d.buf)
	}
	copy(p, o.d.buf)
}

func (o *Output) Close() {
	outs := o.d.outs
	for i, o2 := range outs {
		if o == o2 {
			copy(outs[i:], outs[i+1:])
			o.d.outs = outs[:len(outs)-1]
			break
		}
	}
}
