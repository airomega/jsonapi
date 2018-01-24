package jsonapi

type extendsField struct {
	fieldBase
	id          string
	extendsType string
	included    *map[string]*Node
	sideload    bool
}

func newExtendsField(args []string, fb fieldBase, included *map[string]*Node, sideload bool) (extendsField, error) {
	ef := extendsField{fieldBase: fb}
	if len(args) < 1 {
		return ef, ErrBadJSONAPIStructTag
	}
	ef.extendsType = args[1]
	ef.included = included
	ef.sideload = sideload
	return ef, nil
}

func (ef extendsField) marshal() (map[string]interface{}, error) {
	n, err := visitModelNode(ef.fieldVal.Interface(), ef.included, ef.sideload)
	if err != nil {
		return nil, err
	}

	if n == nil {
		return nil, ErrEmbeddedPtrNotSet
	}

	if n.ID != "" {
		ef.id = n.ID
	}

	return n.Attributes, nil
}

func (ef extendsField) getNodeType() string {
	return ef.extendsType
}

func (ef extendsField) getNodeID() string {
	return ef.id
}
