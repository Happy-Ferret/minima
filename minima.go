package minima

import (
	"fmt"
)

const (
	id = iota
	st
	fl
	in
	bo
)

const (
	panic_varstr  = "prob"
	panic_name    = 1
	lambda_varstr = "lambda"
	lambda_name   = 0
	max_depth     = 50
	true_str      = "true"
	false_str     = "false"
)

type Func struct {
	Vars    *Vars
	Args    []uint
	Com     *Cmd
	Recover *Cmd
	Defers  []*Cmd
}

func (f *Func) Eval(vars *Vars, params *[]interface{}) interface{} {
	nvar := &Vars{Sym: make([]map[uint]interface{}, max_depth), Lev: f.Vars.Lev, Jump: f.Vars.Jump} // Support for recursion.
	copy(nvar.Sym, f.Vars.Sym)
	for i, v := range f.Args {
		nvar.Set(v, (*params)[i])
	}
	v := f.Com.Eval(nvar)
	recovered := false
	if f.Vars.Jump.Type == 2 && f.Recover != nil { // Panic
		recovered = true
		// Think again about attaching a recover to a given Func. Recover command runs every time but it is unnecessary after the first evaluation.
		// Also think about the ugliness of writing data into the Func.
		f.Vars.Jump.Type = 0
		nvar.Lev++ // Hack to inject local var into recover.
		nvar.Set(panic_name, f.Vars.Jump.Dat.(*Panic).Reason)
		nvar.Lev--
		v = f.Recover.Eval(nvar)
	}
	if f.Defers != nil {
		for _, com := range f.Defers {
			if recovered {
				nvar.Lev++ // Hack to inject local var into refers.
				nvar.Set(panic_name, f.Vars.Jump.Dat.(*Panic).Reason)
				nvar.Lev--
			}
			com.Eval(nvar)
		}
	}
	return v
}

type Break struct {
	Lev    int
	RetVal interface{}
}

type Panic struct {
	Reason string
}

type Jump struct {
	Type int // 0 Nothing 1 Break 2 Exc
	Dat  interface{}
}

// I sense some ignorance of multithreading here, but hey, it's just a prototype.
type Vars struct {
	Sym  []map[uint]interface{}
	Lev  int
	Jump *Jump
}

func (v *Vars) Get(varname uint) interface{} {
	var ret interface{}
	for i := v.Lev - 1; i >= 0; i-- {
		if v.Sym[i] != nil && len(v.Sym[i]) > 0 {
			ret, ok := v.Sym[i][varname]
			if ok {
				return ret
			}
		}
	}
	return ret
}

// Equals to = in Go.
func (v *Vars) Mod(varname uint, val interface{}) {
	for i := v.Lev - 1; i >= 0; i-- {
		if v.Sym[i] != nil && len(v.Sym[i]) > 0 {
			_, ok := v.Sym[i][varname]
			if ok {
				v.Sym[i][varname] = val
			}
		}
	}
}

// Equals to := in Go.
func (v *Vars) Set(varname uint, val interface{}) {
	if v.Sym[v.Lev-1] == nil {
		v.Sym[v.Lev-1] = map[uint]interface{}{}
	}
	v.Sym[v.Lev-1][varname] = val
}

type Cmd struct {
	Op         string
	IDName     uint
	Builtin    int
	Params     []*Cmd
	ParentCmd  *Cmd  // Both ParentCmd and ParentFunc here just to support panics or panic-like magic.
	ParentFunc *Func //
	Kind       int
	Value      interface{}
}

// TODO: refactor code to get rid of a lot of evaling inside builtins.
// A var should eval to it's value, a constant to a const etc...
func (c *Cmd) Eval(vars *Vars) interface{} {
	if vars.Jump.Type != 0 {
		return nil
	}
	var v interface{}
	if c.Params != nil {
		vars.Lev++
		if c.Builtin > 0 {
			v = builtins[c.Builtin-1](c, vars)
		} else {
			fun := vars.Get(c.IDName)
			if val, k := fun.(*Func); k {
				params := make([]interface{}, 0, 10)
				for _, va := range c.Params {
					params = append(params, va.Eval(vars))
				}
				v = val.Eval(vars, &params)
			} else {
				if _, isF := fun.(Func); isF {
					panic("Somewhere there is a Func set instead of *Func, name: " + c.Op)
				}
				fmt.Println(c.IDName)
				panic("Call of non-function " + c.Op)
			}
		}
		vars.Sym[vars.Lev] = nil
		vars.Lev--
	} else {
		if c.Kind == id {
			v = vars.Get(c.IDName)
		} else {
			v = c.Value
		}
	}
	return v
}

func (c *Cmd) Add(vars *Vars) interface{} {
	var res int
	for _, v := range c.Params {
		res += v.Eval(vars).(int)
	}
	return res
}

func (c *Cmd) And(vars *Vars) interface{} {
	for _, v := range c.Params {
		val := v.Eval(vars)
		if value, _ := val.(bool); value == false {
			return false
		}
	}
	if len(c.Params) == 0 {
		return false
	}
	return true
}

func (c *Cmd) Break(vars *Vars) interface{} {
	b := &Break{Lev: 1}
	l := len(c.Params)
	if l == 2 {
		b.RetVal = c.Params[1].Eval(vars)
	}
	if l == 1 {
		b.Lev = c.Params[0].Eval(vars).(int)
	}
	vars.Jump.Type = 1
	vars.Jump.Dat = b
	return nil
}

func (c *Cmd) Defer(vars *Vars) interface{} {
	p := c
	for {
		if p.ParentFunc != nil {
			if p.ParentFunc.Defers == nil {
				p.ParentFunc.Defers = []*Cmd{c.Params[0]}
			} else {
				p.ParentFunc.Defers = append([]*Cmd{c.Params[0]}, p.ParentFunc.Defers...)
			}
			break
		}
		if p.ParentCmd == nil {
			break
		}
		p = p.ParentCmd
	}
	return nil
}

func (c *Cmd) Div(vars *Vars) interface{} {
	res := c.Params[0].Eval(vars).(int)
	for i := 1; i < len(c.Params); i++ {
		res /= c.Params[i].Eval(vars).(int)
	}
	return res
}

func (c *Cmd) Eq(vars *Vars) interface{} {
	return c.Params[0].Eval(vars).(int) == c.Params[1].Eval(vars).(int)
}

func (c *Cmd) For(vars *Vars) interface{} {
	v, k := kind(c.Params[0].Op)
	var val int
	if k == in {
		val = v.(int)
	} else if k == id {
		val = vars.Get(c.Params[0].IDName).(int)
	}
	for i := 0; i < val; i++ {
		if vars.Jump.Type != 0 {
			if vars.Jump.Type == 1 {
				b, _ := vars.Jump.Dat.(*Break)
				if b.Lev == 1 {
					vars.Jump.Type = 0
					vars.Jump.Dat = nil
					return b.RetVal
				} else {
					b.Lev--
					return nil
				}
			} else {
				return nil
			}
		}
		if i == val-1 {
			return c.Params[1].Eval(vars)
		}
		c.Params[1].Eval(vars)
	}
	return nil
}

func (c *Cmd) Func(vars *Vars) interface{} {
	var name uint
	co := 0
	if c.Params[0].Params == nil {
		name = c.Params[0].IDName
		co++
	} else {
		name = lambda_name
	}
	nvar := &Vars{Sym: make([]map[uint]interface{}, max_depth), Lev: vars.Lev, Jump: vars.Jump} // TODO: think about the Lev+1 later.
	copy(nvar.Sym, vars.Sym)
	f := Func{Vars: nvar}
	if len(c.Params) == co+2 { // Has parameters.
		args := []uint{c.Params[co].IDName}
		for _, v := range c.Params[co].Params {
			args = append(args, v.IDName)
		}
		f.Args = args
		co++
	}
	f.Com = c.Params[co]
	c.Params[co].ParentFunc = &f // To support panics.
	vars.Set(name, &f)           // Not sure if it will be kept.
	f.Vars.Set(name, &f)
	f.Vars.Lev++
	// TODO: think about the possible inconsistency what a nils cause when we imagine vars as a []map[string]interface{} in terms of references.
	// For example: x := make(map[string]interface{}, 10); copying it to a new slice and Vars.Setting variables assuming that both will updated will only work
	// if the maps are already existing and not nil.
	return &f // f instead of &f was a source of "Somewhere..." etc panic
}

func (c *Cmd) Get(vars *Vars) interface{} {
	return vars.Get(c.Params[0].IDName)
}

// TODO: current syntax rules makes a the second and third param of if (call anything) tp ((call anything)) which means (run (call anything))
// Same applies to for and all "cotrol structures".
func (c *Cmd) If(vars *Vars) interface{} {
	v := c.Params[0].Eval(vars)
	var ret interface{}
	if v.(bool) {
		ret = c.Params[1].Eval(vars)
	} else if len(c.Params) > 2 && vars.Jump.Type == 0 {
		ret = c.Params[2].Eval(vars)
	}
	return ret
}

func (c *Cmd) Less(vars *Vars) interface{} {
	return c.Params[0].Eval(vars).(int) < c.Params[1].Eval(vars).(int)
}

func (c *Cmd) List(vars *Vars) interface{} {
	return nil
}

func (c *Cmd) Map(vars *Vars) interface{} {
	return nil
}

func (c *Cmd) Mod(vars *Vars) interface{} {
	vname := c.Params[0].IDName
	var v interface{}
	v = c.Params[1].Eval(vars)
	vars.Mod(vname, v)
	return v
}

func (c *Cmd) Mul(vars *Vars) interface{} {
	res := 1
	for _, v := range c.Params {
		res *= v.Eval(vars).(int)
	}
	return res
}

func (c *Cmd) Or(vars *Vars) interface{} {
	for _, v := range c.Params {
		val := v.Eval(vars)
		if value, _ := val.(bool); value == true {
			return true
		}
	}
	return false
}

func (c *Cmd) Panic(vars *Vars) interface{} {
	p := &Panic{}
	if len(c.Params) == 1 {
		p.Reason = c.Params[0].Eval(vars).(string)
	}
	vars.Jump.Type = 2
	vars.Jump.Dat = p
	return nil
}

func (c *Cmd) Print(vars *Vars) interface{} {
	l := len(c.Params)
	for i, v := range c.Params {
		val := v.Eval(vars)
		fmt.Print(val)
		if i == l-1 {
			return val
		}
	}
	return nil
}

func (c *Cmd) Println(vars *Vars) interface{} {
	v := c.Print(vars)
	fmt.Print("\n")
	return v
}

func (c *Cmd) Read(vars *Vars) interface{} {
	return nil
}

func (c *Cmd) Recover(vars *Vars) interface{} {
	p := c
	for {
		if p.ParentFunc != nil {
			p.ParentFunc.Recover = c.Params[0]
			break
		}
		if p.ParentCmd == nil {
			break
		}
		p = p.ParentCmd
	}
	return nil
}

func (c *Cmd) Run(vars *Vars) interface{} {
	if len(c.Params) == 1 {
		return c.Params[0].Eval(vars)
	}
	l := len(c.Params)
	for i, v := range c.Params {
		if vars.Jump.Type != 0 {
			if vars.Jump.Type == 2 && c.ParentCmd == nil {
				panic(vars.Jump.Dat.(*Panic).Reason)
			}
			return nil
		}
		if i == l-1 {
			return v.Eval(vars)
		}
		v.Eval(vars)
	}
	return nil
}

func (c *Cmd) Set(vars *Vars) interface{} {
	vname := c.Params[0].IDName
	var v interface{}
	v = c.Params[1].Eval(vars)
	vars.Set(vname, v)
	return v
}

func (c *Cmd) Sub(vars *Vars) interface{} {
	var res int
	first := true
	for _, v := range c.Params {
		va := v.Eval(vars).(int)
		if first {
			res = va
		} else {
			res -= va
		}
		first = false
	}
	return res
}
