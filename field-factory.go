package jsonapi

import (
	"reflect"
)

type field interface {
	getFieldName() string
	marshal() (interface{}, error)
}

type fieldBase struct {
	fieldVal  reflect.Value
	fieldType reflect.StructField
}

func newFieldBase(fieldVal reflect.Value, fieldType reflect.StructField) fieldBase {
	return fieldBase{fieldVal: fieldVal, fieldType: fieldType}
}

func getField(args []string, fieldVal reflect.Value, fieldType reflect.StructField, included *map[string]*Node, sideload bool) (field, error) {
	if len(args) < 1 {
		return nil, ErrBadJSONAPIStructTag
	}

	annotation := args[0]

	if (annotation == annotationClientID && len(args) != 1) ||
		(annotation != annotationClientID && len(args) < 2) {
		return nil, ErrBadJSONAPIStructTag
	}

	fb := newFieldBase(fieldVal, fieldType)
	switch annotation {
	case annotationPrimary:
		return newPrimaryField(args[1:], fb)
	case annotationClientID:
		clientID := fb.fieldValue.String()
		if clientID != "" {
			fb.node.ClientID = clientID
		}
	case annotationExtends:
		return newExtendsField(args[1:], fb, included, sideload)
	case annotationAttribute:
		return newAttributeField(args[1:], fb), nil
	case annotationRelation:
		return newRelationsField(args, fb), nil
	default:
		return nil, ErrBadJSONAPIStructTag
	}

}
