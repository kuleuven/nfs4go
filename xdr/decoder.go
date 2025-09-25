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

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

type Decoder struct {
	r io.Reader

	buf [8]byte
}

func (d *Decoder) String() (string, error) {
	data, err := d.Bytes()
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func (d *Decoder) Bytes() ([]byte, error) {
	size, err := d.Uint32()
	if err != nil {
		return nil, err
	}

	n := int(size)

	b := make([]byte, n+Pad(n))

	_, err = io.ReadFull(d.r, b)
	if err != nil {
		return nil, err
	}

	return b[:n], nil
}

func (d *Decoder) ByteArray(buf []byte) error {
	_, err := io.ReadFull(d.r, buf)
	if err != nil {
		return err
	}

	if padding := Pad(len(buf)); padding > 0 {
		_, err = io.ReadFull(d.r, d.buf[:padding])
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Decoder) Uint32() (uint32, error) {
	b := d.buf[:4]

	_, err := io.ReadFull(d.r, b)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint32(b), nil
}

func (d *Decoder) Uint64() (uint64, error) {
	b := d.buf[:8]

	_, err := io.ReadFull(d.r, b)
	if err != nil {
		return 0, err
	}

	return binary.BigEndian.Uint64(b), nil
}

func (d *Decoder) Bool() (bool, error) {
	b, err := d.Uint32()
	if err != nil {
		return false, err
	}

	return b != 0, nil
}

func (d *Decoder) Float32() (float32, error) {
	v, err := d.Uint32()
	if err != nil {
		return 0, err
	}

	return math.Float32frombits(v), nil
}

func (d *Decoder) Float64() (float64, error) {
	v, err := d.Uint64()
	if err != nil {
		return 0, err
	}

	return math.Float64frombits(v), nil
}

func (d *Decoder) Uint32s() ([]uint32, error) {
	size, err := d.Uint32()
	if err != nil {
		return nil, err
	}

	n := int(size)

	b := make([]uint32, n)

	for i := range n {
		b[i], err = d.Uint32()
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

func (d *Decoder) Strings() ([]string, error) {
	size, err := d.Uint32()
	if err != nil {
		return nil, err
	}

	n := int(size)

	b := make([]string, n)

	for i := range n {
		b[i], err = d.String()
		if err != nil {
			return nil, err
		}
	}

	return b, nil
}

// Decodable is a type that can be decoded from an xdr.Decoder
// A struct should be decodable without implementing this interface,
// but for some structs it pays of to avoid reflect calls by
// implementing this interface.
type Decodable interface {
	Decode(decoder *Decoder) error
}

func (d *Decoder) Decode(target interface{}) error {
	var err error

	switch v := target.(type) {
	case *int:
		var i uint32

		i, err = d.Uint32()

		*v = int(i)
	case *uint32:
		*v, err = d.Uint32()
	case *uint64:
		*v, err = d.Uint64()
	case *string:
		*v, err = d.String()
	case *bool:
		*v, err = d.Bool()
	case *float32:
		*v, err = d.Float32()
	case *float64:
		*v, err = d.Float64()
	case *[]byte:
		*v, err = d.Bytes()
	case *[16]byte:
		err = d.ByteArray((*v)[:])
	case *[]uint32:
		*v, err = d.Uint32s()
	case *[3]uint32:
		err = d.DecodeAll(&(v[0]), &(v[1]), &(v[2]))
	case *[]string:
		*v, err = d.Strings()
	case Decodable:
		err = v.Decode(d)
	default:
		val := reflect.ValueOf(target)

		if !val.IsValid() {
			return errors.New("invalid target")
		}

		kind := val.Kind()
		if kind != reflect.Ptr {
			return errors.New("ReadAs: expects a ptr target")
		}

		logrus.Warnf("decoding using reflect: %T", target)

		err = d.decodeReflect(val)
	}

	return err
}

func (d *Decoder) DecodeAll(target ...interface{}) error {
	for _, t := range target {
		if err := d.Decode(t); err != nil {
			return err
		}
	}

	return nil
}

func (d *Decoder) Union(mode *uint32, args ...interface{}) error {
	var err error

	*mode, err = d.Uint32()
	if err != nil {
		return err
	}

	fieldCount := len(args)

	if int(*mode) >= fieldCount {
		return fmt.Errorf("invalid union mode: %d", *mode)
	}

	return d.Decode(args[int(*mode)])
}

func (d *Decoder) decodeReflect(v reflect.Value) error { //nolint:funlen,gocognit,gocyclo
	kind := v.Elem().Kind()

	switch kind { //nolint:exhaustive
	case reflect.Ptr:
		hasValue, err := d.Bool()
		if err != nil {
			return err
		}

		if !hasValue {
			return nil
		}

		ev := v.Elem()

		if ev.IsNil() {
			ev.Set(reflect.New(ev.Type().Elem()))
		}

		return d.decodeReflect(ev)

	case reflect.Bool:
		value, err := d.Bool()
		if err != nil {
			return err
		}

		v.Elem().SetBool(value)

		return nil

	case reflect.Float32:
		value, err := d.Float32()
		if err != nil {
			return err
		}

		v.Elem().Set(reflect.ValueOf(value))

		return nil

	case reflect.Float64:
		value, err := d.Float64()
		if err != nil {
			return err
		}

		v.Elem().SetFloat(value)

		return nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32:
		value, err := d.Uint32()
		if err != nil {
			return err
		}

		v.Elem().SetUint(uint64(value))

		return nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		value, err := d.Uint32()
		if err != nil {
			return err
		}

		v.Elem().SetInt(int64(value))

		return nil

	case reflect.Int64, reflect.Uint64:
		value, err := d.Uint64()
		if err != nil {
			return err
		}

		vFrom := reflect.ValueOf(value)
		vTo := v.Elem()

		if vFrom.Type().AssignableTo(vTo.Type()) {
			v.Elem().SetUint(value)

			return nil
		}

		if vFrom.Type().ConvertibleTo(vTo.Type()) {
			v.Elem().Set(vFrom.Convert(vTo.Type()))

			return nil
		}

		return fmt.Errorf("unable to assign %T to %s", value, kind)

	case reflect.Array: // fixed length array
		vtyp := v.Elem().Type()
		arrLen := vtyp.Len()

		// Special case: [n]byte
		if vtyp.Elem().Kind() == reflect.Uint8 {
			byteSlice := make([]byte, arrLen)

			err := d.ByteArray(byteSlice)
			if err != nil {
				return err
			}

			for i, bv := range byteSlice {
				v.Elem().Index(i).Set(reflect.ValueOf(bv))
			}

			return nil
		}

		varr := reflect.New(vtyp)

		for i := range arrLen {
			item := reflect.New(vtyp.Elem())

			if err := d.decodeReflect(item); err != nil {
				return err
			}

			varr.Elem().Index(i).Set(item.Elem())
		}

		v.Elem().Set(varr.Elem())

		return nil

	case reflect.Slice:
		vtyp := v.Elem().Type()

		// Special case: []byte
		if vtyp.Elem().Kind() == reflect.Uint8 {
			dat, err := d.Bytes()
			if err != nil {
				return err
			}

			v.Elem().SetBytes(dat)

			return nil
		}

		arrLen32, err := d.Uint32()
		if err != nil {
			return err
		}

		arrLen := int(arrLen32)

		varr := reflect.MakeSlice(vtyp, arrLen, arrLen)

		for i := range arrLen {
			// read an element
			item := reflect.New(vtyp.Elem())

			if err := d.decodeReflect(item); err != nil {
				return err
			}

			v1 := varr.Index(i)
			v1.Set(item.Elem())
		}

		v.Elem().Set(varr)

		return nil

	case reflect.String:
		dat, err := d.String()
		if err != nil {
			return err
		}

		v.Elem().SetString(dat)

		return nil

	case reflect.Struct:
		vtyp := v.Elem().Type()
		fieldCount := vtyp.NumField()

		if fieldCount > 0 && vtyp.Field(0).Tag.Get("xdr") == "union" {
			return d.readUnion(v)
		}

		for i := range fieldCount {
			field := vtyp.Field(i)
			fv := v.Elem().Field(i)

			pToFv := fv.Addr()

			if fv.Type().Kind() == reflect.Slice && fv.IsNil() {
				pToFv = reflect.New(fv.Type())
			}

			if err := d.decodeReflect(pToFv); err != nil {
				return fmt.Errorf("ReadValue(field:%s): %v", field.Name, err)
			}

			fv.Set(pToFv.Elem())
		}

		return nil

	default:
		return fmt.Errorf("type not supported: %s", kind)
	}
}

func (d *Decoder) readUnion(v reflect.Value) error {
	mode, err := d.Uint32()
	if err != nil {
		return err
	}

	v.Elem().Field(0).SetUint(uint64(mode))

	fieldCount := v.Elem().Type().NumField()

	if int(mode)+1 >= fieldCount {
		return fmt.Errorf("invalid union mode: %d", mode)
	}

	fv := v.Elem().Field(int(mode) + 1).Addr()

	return d.decodeReflect(fv)
}

func (d *Decoder) Read(target interface{}) (int, error) {
	r := &CountReader{
		Reader: d.r,
	}

	err := NewDecoder(r).Decode(target)

	return r.Count(), err
}

type CountReader struct {
	Reader io.Reader
	n      int
}

func NewCountReader(r io.Reader) *CountReader {
	return &CountReader{Reader: r}
}

func (c *CountReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	c.n += n

	return n, err
}

func (c *CountReader) Count() int {
	return c.n
}
