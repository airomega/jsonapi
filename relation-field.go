package jsonapi

import (
	"fmt"
	"reflect"
)

type relationField struct {
	fieldBase
	relationName string
	omitEmpty    bool
	included     *map[string]*Node
	sideload     bool
}

func newRelationsField(args []string, fb fieldBase, included *map[string]*Node, sideload bool) relationField {
	rf := relationField{fieldBase: fb}
	rf.relationName = args[0]
	if len(args) > 1 {
		rf.omitEmpty = args[1] == annotationOmitEmpty
	}
	rf.included = included
	rf.sideload = sideload
	return rf
}

func (rf relationField) marshal() (interface{}, error) {
	if rf.relationName == "" {
		return nil, ErrBadJSONAPIStructTag
	}

	isSlice := rf.fieldVal.Type().Kind() == reflect.Slice
	if rf.omitEmpty &&
		(isSlice && rf.fieldVal.Len() < 1 ||
			(!isSlice && rf.fieldVal.IsNil())) {
		return nil, nil
	}
	return nil, nil

	/*var relLinks *Links
	if linkableModel, ok := model.(RelationshipLinkable); ok {
		relLinks = linkableModel.JSONAPIRelationshipLinks(args[1])
	}

	var relMeta *Meta
	if metableModel, ok := model.(RelationshipMetable); ok {
		relMeta = metableModel.JSONAPIRelationshipMeta(fb.args[1])
	}

	if isSlice {
		// to-many relationship
		relationship, err := visitModelNodeRelationships(
			rf.fieldVal,
			rf.included,
			rf.sideload,
		)
		if err != nil {
			return nil, err
		}
		relationship.Links = relLinks
		relationship.Meta = relMeta

		if rf.sideload {
			shallowNodes := []*Node{}
			for _, n := range relationship.Data {
				appendIncluded(rf.included, n)
				shallowNodes = append(shallowNodes, toShallowNode(n))
			}

			node.Relationships[fb.args[1]] = &RelationshipManyNode{
				Data:  shallowNodes,
				Links: relationship.Links,
				Meta:  relationship.Meta,
			}
		} else {
			node.Relationships[fb.args[1]] = relationship
		}
	} else {
		// to-one relationships

		// Handle null relationship case
		if rf.fieldVal.IsNil() {
			node.Relationships[fb.args[1]] = &RelationshipOneNode{Data: nil}
			return nil
		}

		relationship, err := visitModelNode(
			rf.fieldVal.Interface(),
			rf.included,
			rf.sideload,
		)
		if err != nil {
			return nil, err
		}

		if rf.sideload {
			appendIncluded(fb.included, relationship)
			node.Relationships[fb.args[1]] = &RelationshipOneNode{
				Data:  toShallowNode(relationship),
				Links: relLinks,
				Meta:  relMeta,
			}
		} else {
			node.Relationships[fb.args[1]] = &RelationshipOneNode{
				Data:  relationship,
				Links: relLinks,
				Meta:  relMeta,
			}
		}
	}
	return nil*/
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
