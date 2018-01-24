package jsonapi

import (
	"reflect"
	"strconv"
)

type primaryField struct {
	fieldBase
	nodeType string
}

func newPrimaryField(args []string, fb fieldBase) (primaryField, error) {
	pf := primaryField{fieldBase: fb}
	if len(args) < 1 {
		return pf, ErrBadJSONAPIStructTag
	}

	pf.nodeType = args[1]
	return pf, nil
}

func (pf primaryField) marshal(node *Node) error {
	if node.ID == "" {
		return nil
	}

	// Check the JSON API Type
	node.Type = pf.nodeType

	// ID will have to be transmitted as astring per the JSON API spec
	v := reflect.ValueOf(node.ID)

	// Deal with PTRS
	var kind reflect.Kind
	if pf.fieldVal.Kind() == reflect.Ptr {
		kind = pf.fieldType.Type.Elem().Kind()
	} else {
		kind = pf.fieldType.Type.Kind()
	}

	// Handle String case
	if kind == reflect.String {
		assign(pf.fieldVal, v)
		return nil
	}

	// Value was not a string... only other supported type was a numeric,
	// which would have been sent as a float value.
	floatValue, err := strconv.ParseFloat(node.ID, 64)
	if err != nil {
		// Could not convert the value in the "id" attr to a float
		return ErrBadJSONAPIID
	}

	// Convert the numeric float to one of the supported ID numeric types
	// (int[8,16,32,64] or uint[8,16,32,64])
	var idValue reflect.Value
	switch kind {
	case reflect.Int:
		n := int(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Int8:
		n := int8(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Int16:
		n := int16(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Int32:
		n := int32(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Int64:
		n := int64(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Uint:
		n := uint(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Uint8:
		n := uint8(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Uint16:
		n := uint16(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Uint32:
		n := uint32(floatValue)
		idValue = reflect.ValueOf(&n)
	case reflect.Uint64:
		n := uint64(floatValue)
		idValue = reflect.ValueOf(&n)
	default:
		// We had a JSON float (numeric), but our field was not one of the
		// allowed numeric types
		return ErrBadJSONAPIID
	}

	assign(pf.fieldVal, idValue)
	return nil
}
