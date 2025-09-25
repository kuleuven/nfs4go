package xdr

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"

	"github.com/sirupsen/logrus"
)

func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

type Encoder struct {
	w io.Writer

	zero [4]byte
}

func (e *Encoder) String(v string) error {
	return e.Bytes([]byte(v))
}

func (e *Encoder) Uint32(v uint32) error {
	return binary.Write(e.w, binary.BigEndian, v)
}

func (e *Encoder) Uint64(v uint64) error {
	return binary.Write(e.w, binary.BigEndian, v)
}

func (e *Encoder) Bool(v bool) error {
	value := uint32(0)

	if v {
		value = 1
	}

	return e.Uint32(value)
}

func (e *Encoder) Float32(v float32) error {
	return e.Uint32(math.Float32bits(v))
}

func (e *Encoder) Float64(v float64) error {
	return e.Uint64(math.Float64bits(v))
}

func (e *Encoder) Bytes(v []byte) error {
	size := len(v)

	if err := e.Uint32(uint32(size)); err != nil {
		return err
	}

	_, err := e.w.Write(v)
	if err != nil {
		return err
	}

	if padding := Pad(size); padding > 0 {
		_, err = e.w.Write(e.zero[:padding])
	}

	return err
}

func (e *Encoder) ByteArray(v []byte) error {
	_, err := e.w.Write(v)
	if err != nil {
		return err
	}

	if padding := Pad(len(v)); padding > 0 {
		_, err = e.w.Write(e.zero[:padding])
	}

	return err
}

func (e *Encoder) Uint32s(v []uint32) error {
	size := len(v)

	if err := e.Uint32(uint32(size)); err != nil {
		return err
	}

	for _, v := range v {
		if err := e.Uint32(v); err != nil {
			return err
		}
	}

	return nil
}

func (e *Encoder) Strings(v []string) error {
	size := len(v)

	if err := e.Uint32(uint32(size)); err != nil {
		return err
	}

	for _, v := range v {
		if err := e.String(v); err != nil {
			return err
		}
	}

	return nil
}

// Encodable is a type that can be encoded with an xdr.Encoder
// A struct should be encodable without implementing this interface,
// but for some structs it pays of to avoid reflect calls by
// implementing this interface.
type Encodable interface {
	Encode(encoder *Encoder) error
}

func (e *Encoder) Encode(obj interface{}) error {
	switch v := obj.(type) {
	case int:
		return e.Uint32(uint32(v))
	case uint32:
		return e.Uint32(v)
	case uint64:
		return e.Uint64(v)
	case string:
		return e.String(v)
	case bool:
		return e.Bool(v)
	case float32:
		return e.Float32(v)
	case float64:
		return e.Float64(v)
	case []byte:
		return e.Bytes(v)
	case [16]byte:
		return e.ByteArray(v[:])
	case []uint32:
		return e.Uint32s(v)
	case [3]uint32:
		return e.EncodeAll(v[0], v[1], v[2])
	case []string:
		return e.Strings(v)
	case Encodable:
		return v.Encode(e)
	default:
	}

	v := reflect.ValueOf(obj)

	if !v.IsValid() {
		return errors.New("invalid object")
	}

	logrus.Warnf("encoding using reflect: %T", obj)

	return e.encodeReflect(v)
}

func (e *Encoder) EncodeAll(src ...interface{}) error {
	for _, v := range src {
		if err := e.Encode(v); err != nil {
			return err
		}
	}

	return nil
}

func (e *Encoder) Union(mode uint32, args ...interface{}) error {
	if err := e.Uint32(mode); err != nil {
		return err
	}

	fieldCount := len(args)

	if int(mode) >= fieldCount {
		return fmt.Errorf("invalid union mode: %d", mode)
	}

	return e.Encode(args[int(mode)])
}

func (e *Encoder) encodeReflect(v reflect.Value) error { //nolint:funlen,gocognit,gocyclo
	kind := v.Type().Kind()

	if kind == reflect.Ptr {
		if v.IsNil() {
			return e.Bool(false)
		}

		if err := e.Bool(true); err != nil {
			return err
		}

		return e.Encode(v.Elem().Interface())
	}

	switch kind { //nolint:exhaustive
	case reflect.Bool:
		return e.Bool(v.Bool())

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		i := uint32(0)
		vTo := reflect.ValueOf(&i)

		if v.Type().ConvertibleTo(vTo.Elem().Type()) {
			v = v.Convert(vTo.Elem().Type())
		}

		if v.Type().AssignableTo(vTo.Elem().Type()) {
			vTo.Elem().Set(v)
		} else {
			return fmt.Errorf("unable to assign %s to %s", v.Type().Name(), vTo.Type().Name())
		}

		return e.Uint32(i)

	case reflect.Int64, reflect.Uint64:
		i := uint64(0)
		vTo := reflect.ValueOf(&i)

		if v.Type().ConvertibleTo(vTo.Elem().Type()) {
			v = v.Convert(vTo.Elem().Type())
		}

		if v.Type().AssignableTo(vTo.Elem().Type()) {
			vTo.Elem().Set(v)
		} else {
			return fmt.Errorf("unable to assign %s to %s", v.Type().Name(), vTo.Type().Name())
		}

		return e.Uint64(i)

	case reflect.Float32:
		return e.Float32(float32(v.Float()))

	case reflect.Float64:
		return e.Float64(v.Float())

	case reflect.Array:
		vElTyp := v.Type().Elem()
		cnt := v.Len()

		if cnt <= 0 {
			return nil
		}

		// Special case: [n]byte
		if vElTyp.Kind() == reflect.Uint8 {
			sTyp := reflect.SliceOf(vElTyp)
			slice := reflect.MakeSlice(sTyp, v.Len(), v.Len())
			reflect.Copy(slice, v)

			return e.ByteArray(slice.Bytes())
		}

		for i := range cnt {
			if err := e.encodeReflect(v.Index(i)); err != nil {
				return err
			}
		}

		return nil

	case reflect.Slice:
		if v.IsNil() {
			return e.Uint32(0)
		}

		vElTyp := v.Type().Elem()

		// Special case: []byte
		if vElTyp.Kind() == reflect.Uint8 {
			return e.Bytes(v.Bytes())
		}

		cnt := v.Len()

		if err := e.Uint32(uint32(cnt)); err != nil {
			return err
		}

		for i := range cnt {
			if err := e.encodeReflect(v.Index(i)); err != nil {
				return err
			}
		}

		return nil

	case reflect.String:
		return e.String(v.String())

	case reflect.Struct:
		fieldCount := v.Type().NumField()

		if fieldCount > 0 && v.Type().Field(0).Tag.Get("xdr") == "union" {
			return e.writeUnion(v)
		}

		for i := range fieldCount {
			if err := e.encodeReflect(v.Field(i)); err != nil {
				return err
			}
		}

		return nil
	}

	return fmt.Errorf("type not supported: %s", v.Type().Name())
}

func (e *Encoder) writeUnion(v reflect.Value) error {
	mode := int(v.Field(0).Uint())

	if err := e.Uint32(uint32(mode)); err != nil {
		return err
	}

	fieldCount := v.Type().NumField()

	if mode+1 >= fieldCount {
		return fmt.Errorf("invalid union mode: %d", mode)
	}

	fv := v.Field(mode + 1)

	return e.encodeReflect(fv)
}

func (e *Encoder) Write(obj interface{}) (int, error) {
	w := &CountWriter{Writer: e.w}

	err := NewEncoder(w).Encode(obj)

	return w.Count(), err
}

type CountWriter struct {
	Writer io.Writer
	n      int
}

func NewCountWriter(w io.Writer) *CountWriter {
	return &CountWriter{Writer: w}
}

func (c *CountWriter) Write(p []byte) (int, error) {
	n, err := c.Writer.Write(p)
	c.n += n

	return n, err
}

func (c *CountWriter) Count() int {
	return c.n
}
