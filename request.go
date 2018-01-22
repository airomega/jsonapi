package jsonapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const (
	unsuportedStructTagMsg = "Unsupported jsonapi tag annotation, %s"
)

var (
	// ErrInvalidTime is returned when a struct has a time.Time type field, but
	// the JSON value was not a unix timestamp integer.
	ErrInvalidTime = errors.New("Only numbers can be parsed as dates, unix timestamps")
	// ErrInvalidISO8601 is returned when a struct has a time.Time type field and includes
	// "iso8601" in the tag spec, but the JSON value was not an ISO8601 timestamp string.
	ErrInvalidISO8601 = errors.New("Only strings can be parsed as dates, ISO8601 timestamps")
	// ErrUnknownFieldNumberType is returned when the JSON value was a float
	// (numeric) but the Struct field was a non numeric type (i.e. not int, uint,
	// float, etc)
	ErrUnknownFieldNumberType = errors.New("The struct field was not of a known number type")
	// ErrUnsupportedPtrType is returned when the Struct field was a pointer but
	// the JSON value was of a different type
	ErrUnsupportedPtrType = errors.New("Pointer type in struct is not supported")
	// ErrInvalidType is returned when the given type is incompatible with the expected type.
	ErrInvalidType = errors.New("Invalid type provided") // I wish we used punctuation.
)

// UnmarshalPayload converts an io into a struct instance using jsonapi tags on
// struct fields. This method supports single request payloads only, at the
// moment. Bulk creates and updates are not supported yet.
//
// Will Unmarshal embedded and sideloaded payloads.  The latter is only possible if the
// object graph is complete.  That is, in the "relationships" data there are type and id,
// keys that correspond to records in the "included" array.
//
// For example you could pass it, in, req.Body and, model, a BlogPost
// struct instance to populate in an http handler,
//
//   func CreateBlog(w http.ResponseWriter, r *http.Request) {
//   	blog := new(Blog)
//
//   	if err := jsonapi.UnmarshalPayload(r.Body, blog); err != nil {
//   		http.Error(w, err.Error(), 500)
//   		return
//   	}
//
//   	// ...do stuff with your blog...
//
//   	w.Header().Set("Content-Type", jsonapi.MediaType)
//   	w.WriteHeader(201)
//
//   	if err := jsonapi.MarshalPayload(w, blog); err != nil {
//   		http.Error(w, err.Error(), 500)
//   	}
//   }
//
//
// Visit https://github.com/google/jsonapi#create for more info.
//
// model interface{} should be a pointer to a struct.
func UnmarshalPayload(in io.Reader, model interface{}) error {
	payload := new(OnePayload)

	if err := json.NewDecoder(in).Decode(payload); err != nil {
		return err
	}

	if payload.Included != nil {
		includedMap := make(map[string]*Node)
		for _, included := range payload.Included {
			key := fmt.Sprintf("%s,%s", included.Type, included.ID)
			includedMap[key] = included
		}

		return unmarshalNode(payload.Data, reflect.ValueOf(model), &includedMap)
	}
	return unmarshalNode(payload.Data, reflect.ValueOf(model), nil)
}

// UnmarshalManyPayload converts an io into a set of struct instances using
// jsonapi tags on the type's struct fields.
func UnmarshalManyPayload(in io.Reader, t reflect.Type) ([]interface{}, error) {
	payload := new(ManyPayload)

	if err := json.NewDecoder(in).Decode(payload); err != nil {
		return nil, err
	}

	models := []interface{}{}         // will be populated from the "data"
	includedMap := map[string]*Node{} // will be populate from the "included"

	if payload.Included != nil {
		for _, included := range payload.Included {
			key := fmt.Sprintf("%s,%s", included.Type, included.ID)
			includedMap[key] = included
		}
	}

	for _, data := range payload.Data {
		model := reflect.New(t.Elem())
		err := unmarshalNode(data, model, &includedMap)
		if err != nil {
			return nil, err
		}
		models = append(models, model.Interface())
	}

	return models, nil
}

type nodeBuilder struct {
	node       *Node
	args       []string
	fieldValue reflect.Value
	fieldType  reflect.StructField
}

func unmarshalNode(node *Node, model reflect.Value, included *map[string]*Node) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("data is not a jsonapi representation of '%v'", model.Type())
		}
	}()

	modelValue := model.Elem()
	modelType := model.Type().Elem()

	for i := 0; i < modelValue.NumField(); i++ {
		fieldType := modelType.Field(i)
		tag := fieldType.Tag.Get("jsonapi")
		if tag == "" {
			continue
		}

		args := strings.Split(tag, ",")

		if len(args) < 1 {
			return ErrBadJSONAPIStructTag
		}

		nb := nodeBuilder{
			node:       node,
			args:       args,
			fieldValue: modelValue.Field(i),
			fieldType:  fieldType,
		}

		if (nb.args[0] == annotationClientID && len(args) != 1) ||
			(nb.args[0] != annotationClientID && len(args) < 2) {
			return ErrBadJSONAPIStructTag
		}

		switch nb.args[0] {
		case annotationPrimary:
			if err := nb.doPrimary(); err != nil {
				return err
			}
		case annotationClientID:
			if nb.node.ClientID == "" {
				continue
			}
			nb.fieldValue.Set(reflect.ValueOf(nb.node.ClientID))
		case annotationAttribute:
			if err := nb.doAttribute(); err != nil {
				return err
			}
		case annotationEmbedded:
			/*if err := nb.doEmbedded(); err != nil {
				return err
			}*/
		case annotationRelation:
			if err := nb.doRelation(included); err != nil {
				return err
			}
		default:
			return fmt.Errorf(unsuportedStructTagMsg, nb.args[0])
		}
	}

	return nil
}

func (nb nodeBuilder) doPrimary() error {
	if nb.node.ID == "" {
		return nil
	}

	// Check the JSON API Type
	if nb.node.Type != nb.args[1] {
		return fmt.Errorf(
			"Trying to Unmarshal an object of type %#v, but %#v does not match",
			nb.node.Type,
			nb.args[1],
		)
	}

	// ID will have to be transmitted as astring per the JSON API spec
	v := reflect.ValueOf(nb.node.ID)

	// Deal with PTRS
	var kind reflect.Kind
	if nb.fieldValue.Kind() == reflect.Ptr {
		kind = nb.fieldType.Type.Elem().Kind()
	} else {
		kind = nb.fieldType.Type.Kind()
	}

	// Handle String case
	if kind == reflect.String {
		assign(nb.fieldValue, v)
		return nil
	}

	// Value was not a string... only other supported type was a numeric,
	// which would have been sent as a float value.
	floatValue, err := strconv.ParseFloat(nb.node.ID, 64)
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

	assign(nb.fieldValue, idValue)
	return nil
}

func (nb nodeBuilder) doAttribute() error {
	attributes := nb.node.Attributes
	if attributes == nil || len(nb.node.Attributes) == 0 {
		return nil
	}

	var iso8601 bool

	if len(nb.args) > 2 {
		for _, arg := range nb.args[2:] {
			if arg == annotationISO8601 {
				iso8601 = true
			}
		}
	}

	val := attributes[nb.args[1]]

	// continue if the attribute was not included in the request
	if val == nil {
		return nil
	}

	v := reflect.ValueOf(val)

	// Handle field of type time.Time
	if nb.fieldValue.Type() == reflect.TypeOf(time.Time{}) {
		if iso8601 {
			var tm string
			if v.Kind() == reflect.String {
				tm = v.Interface().(string)
			} else {
				return ErrInvalidISO8601
			}

			t, err := time.Parse(iso8601TimeFormat, tm)
			if err != nil {
				return ErrInvalidISO8601
			}

			nb.fieldValue.Set(reflect.ValueOf(t))

			return nil
		}

		var at int64

		if v.Kind() == reflect.Float64 {
			at = int64(v.Interface().(float64))
		} else if v.Kind() == reflect.Int {
			at = v.Int()
		} else {
			return ErrInvalidTime
		}

		t := time.Unix(at, 0)

		nb.fieldValue.Set(reflect.ValueOf(t))
		return nil
	}

	if nb.fieldValue.Type() == reflect.TypeOf([]string{}) {
		values := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			values[i] = v.Index(i).Interface().(string)
		}

		nb.fieldValue.Set(reflect.ValueOf(values))
		return nil
	}

	if nb.fieldValue.Type() == reflect.TypeOf(new(time.Time)) {
		if iso8601 {
			var tm string
			if v.Kind() == reflect.String {
				tm = v.Interface().(string)
			} else {
				return ErrInvalidISO8601
			}

			v, err := time.Parse(iso8601TimeFormat, tm)
			if err != nil {
				return ErrInvalidISO8601
			}

			t := &v

			nb.fieldValue.Set(reflect.ValueOf(t))

			return nil
		}

		var at int64

		if v.Kind() == reflect.Float64 {
			at = int64(v.Interface().(float64))
		} else if v.Kind() == reflect.Int {
			at = v.Int()
		} else {
			return ErrInvalidTime
		}

		v := time.Unix(at, 0)
		t := &v

		nb.fieldValue.Set(reflect.ValueOf(t))

		return nil
	}

	// JSON value was a float (numeric)
	if v.Kind() == reflect.Float64 {
		floatValue := v.Interface().(float64)

		// The field may or may not be a pointer to a numeric; the kind var
		// will not contain a pointer type
		var kind reflect.Kind
		if nb.fieldValue.Kind() == reflect.Ptr {
			kind = nb.fieldType.Type.Elem().Kind()
		} else {
			kind = nb.fieldType.Type.Kind()
		}

		var numericValue reflect.Value

		switch kind {
		case reflect.Int:
			n := int(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Int8:
			n := int8(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Int16:
			n := int16(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Int32:
			n := int32(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Int64:
			n := int64(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Uint:
			n := uint(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Uint8:
			n := uint8(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Uint16:
			n := uint16(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Uint32:
			n := uint32(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Uint64:
			n := uint64(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Float32:
			n := float32(floatValue)
			numericValue = reflect.ValueOf(&n)
		case reflect.Float64:
			n := floatValue
			numericValue = reflect.ValueOf(&n)
		default:
			return ErrUnknownFieldNumberType
		}

		assign(nb.fieldValue, numericValue)
		return nil
	}

	// Field was a Pointer type
	if nb.fieldValue.Kind() == reflect.Ptr {
		var concreteVal reflect.Value

		switch cVal := val.(type) {
		case string:
			concreteVal = reflect.ValueOf(&cVal)
		case bool:
			concreteVal = reflect.ValueOf(&cVal)
		case complex64:
			concreteVal = reflect.ValueOf(&cVal)
		case complex128:
			concreteVal = reflect.ValueOf(&cVal)
		case uintptr:
			concreteVal = reflect.ValueOf(&cVal)
		default:
			return ErrUnsupportedPtrType
		}

		if nb.fieldValue.Type() != concreteVal.Type() {
			return ErrUnsupportedPtrType
		}

		nb.fieldValue.Set(concreteVal)
		return nil
	}

	// As a final catch-all, ensure types line up to avoid a runtime panic.
	if nb.fieldValue.Kind() != v.Kind() {
		return ErrInvalidType
	}
	nb.fieldValue.Set(reflect.ValueOf(val))
	return nil
}

func (nb nodeBuilder) doRelation(included *map[string]*Node) error {
	isSlice := nb.fieldValue.Type().Kind() == reflect.Slice

	if nb.node.Relationships == nil || nb.node.Relationships[nb.args[1]] == nil {
		return nil
	}

	if isSlice {
		// to-many relationship
		relationship := new(RelationshipManyNode)

		buf := bytes.NewBuffer(nil)

		json.NewEncoder(buf).Encode(nb.node.Relationships[nb.args[1]])
		json.NewDecoder(buf).Decode(relationship)

		data := relationship.Data
		models := reflect.New(nb.fieldValue.Type()).Elem()

		for _, n := range data {
			m := reflect.New(nb.fieldValue.Type().Elem().Elem())

			if err := unmarshalNode(
				fullNode(n, included),
				m,
				included,
			); err != nil {
				return err

			}

			models = reflect.Append(models, m)
		}

		nb.fieldValue.Set(models)
	} else {
		// to-one relationships
		relationship := new(RelationshipOneNode)

		buf := bytes.NewBuffer(nil)

		json.NewEncoder(buf).Encode(
			nb.node.Relationships[nb.args[1]],
		)
		json.NewDecoder(buf).Decode(relationship)

		/*
			http://jsonapi.org/format/#document-resource-object-relationships
			http://jsonapi.org/format/#document-resource-object-linkage
			relationship can have a data node set to null (e.g. to disassociate the relationship)
			so unmarshal and set fieldValue only if data obj is not null
		*/
		if relationship.Data == nil {
			return nil
		}

		m := reflect.New(nb.fieldValue.Type().Elem())
		if err := unmarshalNode(
			fullNode(relationship.Data, included),
			m,
			included,
		); err != nil {
			return err
		}

		nb.fieldValue.Set(m)

	}
	return nil
}

func fullNode(n *Node, included *map[string]*Node) *Node {
	includedKey := fmt.Sprintf("%s,%s", n.Type, n.ID)

	if included != nil && (*included)[includedKey] != nil {
		return (*included)[includedKey]
	}

	return n
}

// assign will take the value specified and assign it to the field; if
// field is expecting a ptr assign will assign a ptr.
func assign(field, value reflect.Value) {
	if field.Kind() == reflect.Ptr {
		field.Set(value)
	} else {
		field.Set(reflect.Indirect(value))
	}
}
