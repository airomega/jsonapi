package jsonapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrBadJSONAPIStructTag is returned when the Struct field's JSON API
	// annotation is invalid.
	ErrBadJSONAPIStructTag = errors.New("Bad jsonapi struct tag format")
	// ErrBadJSONAPIID is returned when the Struct JSON API annotated "id" field
	// was not a valid numeric type.
	ErrBadJSONAPIID = errors.New(
		"id should be either string, int(8,16,32,64) or uint(8,16,32,64)")
	// ErrExpectedSlice is returned when a variable or argument was expected to
	// be a slice of *Structs; MarshalMany will return this error when its
	// interface{} argument is invalid.
	ErrExpectedSlice = errors.New("models should be a slice of struct pointers")
	// ErrUnexpectedType is returned when marshalling an interface; the interface
	// had to be a pointer or a slice; otherwise this error is returned.
	ErrUnexpectedType = errors.New("models should be a struct pointer or slice of struct pointers")
	// ErrEmbeddedPtrNotSet is returned when marshalling an interface with an embedded interface
	// the embedded interface must not be null or this error is returned
	ErrEmbeddedPtrNotSet = errors.New("embedded pointer is nil")
)

type fieldbuilder struct {
	model interface{}

	node     *Node
	included *map[string]*Node
	sideload bool

	annotation string
	nodeType   string
	args       []string

	fieldValue reflect.Value
	fieldType  reflect.StructField

	linkableModel RelationshipLinkable
	metableModel  RelationshipMetable
}

// MarshalPayload writes a jsonapi response for one or many records. The
// related records are sideloaded into the "included" array. If this method is
// given a struct pointer as an argument it will serialize in the form
// "data": {...}. If this method is given a slice of pointers, this method will
// serialize in the form "data": [...]
//
// One Example: you could pass it, w, your http.ResponseWriter, and, models, a
// ptr to a Blog to be written to the response body:
//
//	 func ShowBlog(w http.ResponseWriter, r *http.Request) {
//		 blog := &Blog{}
//
//		 w.Header().Set("Content-Type", jsonapi.MediaType)
//		 w.WriteHeader(http.StatusOK)
//
//		 if err := jsonapi.MarshalPayload(w, blog); err != nil {
//			 http.Error(w, err.Error(), http.StatusInternalServerError)
//		 }
//	 }
//
// Many Example: you could pass it, w, your http.ResponseWriter, and, models, a
// slice of Blog struct instance pointers to be written to the response body:
//
//	 func ListBlogs(w http.ResponseWriter, r *http.Request) {
//     blogs := []*Blog{}
//
//		 w.Header().Set("Content-Type", jsonapi.MediaType)
//		 w.WriteHeader(http.StatusOK)
//
//		 if err := jsonapi.MarshalPayload(w, blogs); err != nil {
//			 http.Error(w, err.Error(), http.StatusInternalServerError)
//		 }
//	 }
//
func MarshalPayload(w io.Writer, models interface{}) error {
	payload, err := Marshal(models)
	if err != nil {
		return err
	}

	return json.NewEncoder(w).Encode(payload)
}

// Marshal does the same as MarshalPayload except it just returns the payload
// and doesn't write out results. Useful if you use your own JSON rendering
// library.
func Marshal(models interface{}) (Payloader, error) {
	switch vals := reflect.ValueOf(models); vals.Kind() {
	case reflect.Slice:
		m, err := convertToSliceInterface(&models)
		if err != nil {
			return nil, err
		}

		payload, err := marshalMany(m)
		if err != nil {
			return nil, err
		}

		if linkableModels, isLinkable := models.(Linkable); isLinkable {
			jl := linkableModels.JSONAPILinks()
			if er := jl.validate(); er != nil {
				return nil, er
			}
			payload.Links = linkableModels.JSONAPILinks()
		}

		if metableModels, ok := models.(Metable); ok {
			payload.Meta = metableModels.JSONAPIMeta()
		}

		return payload, nil
	case reflect.Ptr:
		// Check that the pointer was to a struct
		if reflect.Indirect(vals).Kind() != reflect.Struct {
			return nil, ErrUnexpectedType
		}
		return marshalOne(models)
	default:
		return nil, ErrUnexpectedType
	}
}

// MarshalPayloadWithoutIncluded writes a jsonapi response with one or many
// records, without the related records sideloaded into "included" array.
// If you want to serialize the relations into the "included" array see
// MarshalPayload.
//
// models interface{} should be either a struct pointer or a slice of struct
// pointers.
func MarshalPayloadWithoutIncluded(w io.Writer, model interface{}) error {
	payload, err := Marshal(model)
	if err != nil {
		return err
	}
	payload.clearIncluded()

	return json.NewEncoder(w).Encode(payload)
}

// marshalOne does the same as MarshalOnePayload except it just returns the
// payload and doesn't write out results. Useful is you use your JSON rendering
// library.
func marshalOne(model interface{}) (*OnePayload, error) {
	included := make(map[string]*Node)
	rootNode, err := visitModelNode(model, &included, true)
	if err != nil {
		return nil, err
	}
	payload := &OnePayload{Data: rootNode}
	payload.Included = nodeMapValues(&included)

	return payload, nil
}

// marshalMany does the same as MarshalManyPayload except it just returns the
// payload and doesn't write out results. Useful is you use your JSON rendering
// library.
func marshalMany(models []interface{}) (*ManyPayload, error) {
	payload := &ManyPayload{
		Data: []*Node{},
	}
	included := map[string]*Node{}

	for _, model := range models {
		node, err := visitModelNode(model, &included, true)
		if err != nil {
			return nil, err
		}
		payload.Data = append(payload.Data, node)
	}
	payload.Included = nodeMapValues(&included)

	return payload, nil
}

// MarshalOnePayloadEmbedded - This method not meant to for use in
// implementation code, although feel free.  The purpose of this
// method is for use in tests.  In most cases, your request
// payloads for create will be embedded rather than sideloaded for
// related records. This method will serialize a single struct
// pointer into an embedded json response. In other words, there
// will be no, "included", array in the json all relationships will
// be serailized inline in the data.
//
// However, in tests, you may want to construct payloads to post
// to create methods that are embedded to most closely resemble
// the payloads that will be produced by the client. This is what
// this method is intended for.
//
// model interface{} should be a pointer to a struct.
func MarshalOnePayloadEmbedded(w io.Writer, model interface{}) error {
	rootNode, err := visitModelNode(model, nil, false)
	if err != nil {
		return err
	}

	payload := &OnePayload{Data: rootNode}

	return json.NewEncoder(w).Encode(payload)
}

func visitModelNode(model interface{}, included *map[string]*Node, sideload bool) (*Node, error) {
	node := new(Node)
	v := reflect.ValueOf(model)
	modelValue := reflect.ValueOf(model).Elem()
	modelType := reflect.ValueOf(model).Type().Elem()

	if v.IsNil() {
		return nil, nil
	}

	for i := 0; i < modelValue.NumField(); i++ {
		structField := modelValue.Type().Field(i)
		tag := structField.Tag.Get(annotationJSONAPI)
		if tag == "" {
			continue
		}

		fb := fieldbuilder{
			model:      model,
			node:       node,
			included:   included,
			sideload:   sideload,
			args:       strings.Split(tag, annotationSeperator),
			fieldValue: modelValue.Field(i),
			fieldType:  modelType.Field(i),
		}

		if len(fb.args) < 1 {
			return nil, ErrBadJSONAPIStructTag
		}

		annotation := fb.args[0]

		if (annotation == annotationClientID && len(fb.args) != 1) ||
			(annotation != annotationClientID && len(fb.args) < 2) {
			return nil, ErrBadJSONAPIStructTag
		}

		switch annotation {
		case annotationPrimary:
			if err := fb.doPrimary(); err != nil {
				return fb.node, err
			}
		case annotationClientID:
			clientID := fb.fieldValue.String()
			if clientID != "" {
				fb.node.ClientID = clientID
			}
		case annotationExtends:
			if err := fb.doExtends(); err != nil {
				return nil, err
			}
		case annotationAttribute:
			fb.doAttribute()
		case annotationRelation:
			if err := fb.doRelation(); err != nil {
				return nil, err
			}
		default:
			return nil, ErrBadJSONAPIStructTag
		}
	}

	if linkableModel, isLinkable := model.(Linkable); isLinkable {
		jl := linkableModel.JSONAPILinks()
		if er := jl.validate(); er != nil {
			return nil, er
		}
		node.Links = linkableModel.JSONAPILinks()
	}

	if metableModel, ok := model.(Metable); ok {
		node.Meta = metableModel.JSONAPIMeta()
	}

	return node, nil
}

func (fb fieldbuilder) doPrimary() error {
	v := fb.fieldValue

	// Deal with PTRS
	var kind reflect.Kind
	if fb.fieldValue.Kind() == reflect.Ptr {
		kind = fb.fieldType.Type.Elem().Kind()
		v = reflect.Indirect(fb.fieldValue)
	} else {
		kind = fb.fieldType.Type.Kind()
	}

	// Handle allowed types
	switch kind {
	case reflect.String:
		fb.node.ID = v.Interface().(string)
	case reflect.Int:
		fb.node.ID = strconv.FormatInt(int64(v.Interface().(int)), 10)
	case reflect.Int8:
		fb.node.ID = strconv.FormatInt(int64(v.Interface().(int8)), 10)
	case reflect.Int16:
		fb.node.ID = strconv.FormatInt(int64(v.Interface().(int16)), 10)
	case reflect.Int32:
		fb.node.ID = strconv.FormatInt(int64(v.Interface().(int32)), 10)
	case reflect.Int64:
		fb.node.ID = strconv.FormatInt(v.Interface().(int64), 10)
	case reflect.Uint:
		fb.node.ID = strconv.FormatUint(uint64(v.Interface().(uint)), 10)
	case reflect.Uint8:
		fb.node.ID = strconv.FormatUint(uint64(v.Interface().(uint8)), 10)
	case reflect.Uint16:
		fb.node.ID = strconv.FormatUint(uint64(v.Interface().(uint16)), 10)
	case reflect.Uint32:
		fb.node.ID = strconv.FormatUint(uint64(v.Interface().(uint32)), 10)
	case reflect.Uint64:
		fb.node.ID = strconv.FormatUint(v.Interface().(uint64), 10)
	default:
		// We had a JSON float (numeric), but our field was not one of the
		// allowed numeric types
		return ErrBadJSONAPIID
	}

	if fb.node.Type == "" {
		fb.node.Type = fb.args[1]
	}
	return nil
}

func (fb fieldbuilder) doAttribute() {
	var omitEmpty, iso8601 bool

	if len(fb.args) > 2 {
		for _, arg := range fb.args[2:] {
			switch arg {
			case annotationOmitEmpty:
				omitEmpty = true
			case annotationISO8601:
				iso8601 = true
			}
		}
	}

	if fb.node.Attributes == nil {
		fb.node.Attributes = make(map[string]interface{})
	}

	if fb.fieldValue.Type() == reflect.TypeOf(time.Time{}) {
		t := fb.fieldValue.Interface().(time.Time)

		if t.IsZero() {
			return
		}

		if iso8601 {
			fb.node.Attributes[fb.args[1]] = t.UTC().Format(iso8601TimeFormat)
		} else {
			fb.node.Attributes[fb.args[1]] = t.Unix()
		}
	} else if fb.fieldValue.Type() == reflect.TypeOf(new(time.Time)) {
		// A time pointer may be nil
		if fb.fieldValue.IsNil() {
			if omitEmpty {
				return
			}

			fb.node.Attributes[fb.args[1]] = nil
		} else {
			tm := fb.fieldValue.Interface().(*time.Time)

			if tm.IsZero() && omitEmpty {
				return
			}

			if iso8601 {
				fb.node.Attributes[fb.args[1]] = tm.UTC().Format(iso8601TimeFormat)
			} else {
				fb.node.Attributes[fb.args[1]] = tm.Unix()
			}
		}
	} else {
		emptyValue := reflect.Zero(fb.fieldValue.Type())

		// See if we need to omit this field
		if omitEmpty && fb.fieldValue.Interface() == emptyValue.Interface() {
			return
		}

		strAttr, ok := fb.fieldValue.Interface().(string)
		if ok {
			fb.node.Attributes[fb.args[1]] = strAttr
		} else {
			fb.node.Attributes[fb.args[1]] = fb.fieldValue.Interface()
		}
	}
}

func (fb fieldbuilder) doExtends() error {
	if fb.node.Attributes == nil {
		fb.node.Attributes = make(map[string]interface{})
	}

	n, err := visitModelNode(fb.fieldValue.Interface(), fb.included, fb.sideload)
	if err != nil {
		return err
	}

	if n == nil {
		return ErrEmbeddedPtrNotSet
	}

	if n.ID != "" {
		fb.node.ID = n.ID
	}

	for k, v := range n.Attributes {
		fb.node.Attributes[k] = v
	}

	fb.node.Type = fb.args[1]
	return nil
}

func (fb fieldbuilder) doRelation() error {
	var omitEmpty bool

	//add support for 'omitempty' struct tag for marshaling as absent
	if len(fb.args) > 2 {
		omitEmpty = fb.args[2] == annotationOmitEmpty
	}

	isSlice := fb.fieldValue.Type().Kind() == reflect.Slice
	if omitEmpty &&
		(isSlice && fb.fieldValue.Len() < 1 ||
			(!isSlice && fb.fieldValue.IsNil())) {
		return nil
	}

	if fb.node.Relationships == nil {
		fb.node.Relationships = make(map[string]interface{})
	}

	var relLinks *Links
	if linkableModel, ok := fb.model.(RelationshipLinkable); ok {
		relLinks = linkableModel.JSONAPIRelationshipLinks(fb.args[1])
	}

	var relMeta *Meta
	if metableModel, ok := fb.model.(RelationshipMetable); ok {
		relMeta = metableModel.JSONAPIRelationshipMeta(fb.args[1])
	}

	if isSlice {
		// to-many relationship
		relationship, err := visitModelNodeRelationships(
			fb.fieldValue,
			fb.included,
			fb.sideload,
		)
		if err != nil {
			return err
		}
		relationship.Links = relLinks
		relationship.Meta = relMeta

		if fb.sideload {
			shallowNodes := []*Node{}
			for _, n := range relationship.Data {
				appendIncluded(fb.included, n)
				shallowNodes = append(shallowNodes, toShallowNode(n))
			}

			fb.node.Relationships[fb.args[1]] = &RelationshipManyNode{
				Data:  shallowNodes,
				Links: relationship.Links,
				Meta:  relationship.Meta,
			}
		} else {
			fb.node.Relationships[fb.args[1]] = relationship
		}
	} else {
		// to-one relationships

		// Handle null relationship case
		if fb.fieldValue.IsNil() {
			fb.node.Relationships[fb.args[1]] = &RelationshipOneNode{Data: nil}
			return nil
		}

		relationship, err := visitModelNode(
			fb.fieldValue.Interface(),
			fb.included,
			fb.sideload,
		)
		if err != nil {
			return err
		}

		if fb.sideload {
			appendIncluded(fb.included, relationship)
			fb.node.Relationships[fb.args[1]] = &RelationshipOneNode{
				Data:  toShallowNode(relationship),
				Links: relLinks,
				Meta:  relMeta,
			}
		} else {
			fb.node.Relationships[fb.args[1]] = &RelationshipOneNode{
				Data:  relationship,
				Links: relLinks,
				Meta:  relMeta,
			}
		}
	}
	return nil
}

func toShallowNode(node *Node) *Node {
	return &Node{
		ID:   node.ID,
		Type: node.Type,
	}
}

func visitModelNodeRelationships(models reflect.Value, included *map[string]*Node,
	sideload bool) (*RelationshipManyNode, error) {
	nodes := []*Node{}

	for i := 0; i < models.Len(); i++ {
		n := models.Index(i).Interface()

		node, err := visitModelNode(n, included, sideload)
		if err != nil {
			return nil, err
		}

		nodes = append(nodes, node)
	}

	return &RelationshipManyNode{Data: nodes}, nil
}

func appendIncluded(m *map[string]*Node, nodes ...*Node) {
	included := *m

	for _, n := range nodes {
		k := fmt.Sprintf("%s,%s", n.Type, n.ID)

		if _, hasNode := included[k]; hasNode {
			continue
		}

		included[k] = n
	}
}

func nodeMapValues(m *map[string]*Node) []*Node {
	mp := *m
	nodes := make([]*Node, len(mp))

	i := 0
	for _, n := range mp {
		nodes[i] = n
		i++
	}

	return nodes
}

func convertToSliceInterface(i *interface{}) ([]interface{}, error) {
	vals := reflect.ValueOf(*i)
	if vals.Kind() != reflect.Slice {
		return nil, ErrExpectedSlice
	}
	var response []interface{}
	for x := 0; x < vals.Len(); x++ {
		response = append(response, vals.Index(x).Interface())
	}
	return response, nil
}
