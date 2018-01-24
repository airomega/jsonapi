package jsonapi

import (
	"reflect"
	"time"
)

type attributeField struct {
	fieldBase
	fieldName string
	omitEmpty bool
	iso8601   bool
}

func newAttributeField(args []string, fb fieldBase) attributeField {
	af := attributeField{fieldBase: fb, fieldName: args[0]}
	if len(args[1:]) > 0 {
		for _, arg := range args[1:] {
			switch arg {
			case annotationOmitEmpty:
				af.omitEmpty = true
			case annotationISO8601:
				af.iso8601 = true
			}
		}
	}
	return af
}

func (af attributeField) getFieldName() string {
	return af.fieldName
}

func (af attributeField) marshal() (interface{}, error) {
	var omitEmpty, iso8601 bool

	if af.fieldVal.Type() == reflect.TypeOf(time.Time{}) {
		t := af.fieldVal.Interface().(time.Time)

		if t.IsZero() {
			return nil, nil
		}

		if iso8601 {
			return t.UTC().Format(iso8601TimeFormat), nil
		} else {
			return t.Unix(), nil
		}
	} else if af.fieldVal.Type() == reflect.TypeOf(new(time.Time)) {
		// A time pointer may be nil
		if af.fieldVal.IsNil() {
			return nil, nil
		} else {
			tm := af.fieldVal.Interface().(*time.Time)

			if tm.IsZero() && omitEmpty {
				return nil, nil
			}

			if iso8601 {
				return tm.UTC().Format(iso8601TimeFormat), nil
			} else {
				return tm.Unix(), nil
			}
		}
	} else {
		emptyValue := reflect.Zero(af.fieldVal.Type())

		// See if we need to omit this field
		if omitEmpty && af.fieldVal.Interface() == emptyValue.Interface() {
			return nil, nil
		}

		strAttr, ok := af.fieldVal.Interface().(string)
		if ok {
			return strAttr, nil
		} else {
			return af.fieldVal.Interface(), nil
		}
	}
}
