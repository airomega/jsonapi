package jsonapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestMarshalPayload(t *testing.T) {
	book := &Book{ID: 1}
	books := []*Book{book, &Book{ID: 2}}
	var jsonData map[string]interface{}

	// One
	out1 := bytes.NewBuffer(nil)
	MarshalPayload(out1, book)

	if err := json.Unmarshal(out1.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	if _, ok := jsonData["data"].(map[string]interface{}); !ok {
		t.Fatalf("data key did not contain an Hash/Dict/Map")
	}

	// Many
	out2 := bytes.NewBuffer(nil)
	MarshalPayload(out2, books)

	if err := json.Unmarshal(out2.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	if _, ok := jsonData["data"].([]interface{}); !ok {
		t.Fatalf("data key did not contain an Array")
	}
}

func TestMarshal_attrStringSlice(t *testing.T) {
	tags := []string{"fiction", "sale"}
	b := &Book{ID: 1, Tags: tags}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, b); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}

	jsonTags := jsonData["data"].(map[string]interface{})["attributes"].(map[string]interface{})["tags"].([]interface{})
	if e, a := len(tags), len(jsonTags); e != a {
		t.Fatalf("Was expecting tags of length %d got %d", e, a)
	}

	// Convert from []interface{} to []string
	jsonTagsStrings := []string{}
	for _, tag := range jsonTags {
		jsonTagsStrings = append(jsonTagsStrings, tag.(string))
	}

	// Sort both
	sort.Strings(jsonTagsStrings)
	sort.Strings(tags)

	for i, tag := range tags {
		if e, a := tag, jsonTagsStrings[i]; e != a {
			t.Fatalf("At index %d, was expecting %s got %s", i, e, a)
		}
	}
}

func TestWithoutOmitsEmptyAnnotationOnRelation(t *testing.T) {
	blog := &Blog{}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, blog); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	relationships := jsonData["data"].(map[string]interface{})["relationships"].(map[string]interface{})

	// Verifiy the "posts" relation was an empty array
	posts, ok := relationships["posts"]
	if !ok {
		t.Fatal("Was expecting the data.relationships.posts key/value to have been present")
	}
	postsMap, ok := posts.(map[string]interface{})
	if !ok {
		t.Fatal("data.relationships.posts was not a map")
	}
	postsData, ok := postsMap["data"]
	if !ok {
		t.Fatal("Was expecting the data.relationships.posts.data key/value to have been present")
	}
	postsDataSlice, ok := postsData.([]interface{})
	if !ok {
		t.Fatal("data.relationships.posts.data was not a slice []")
	}
	if len(postsDataSlice) != 0 {
		t.Fatal("Was expecting the data.relationships.posts.data value to have been an empty array []")
	}

	// Verifiy the "current_post" was a null
	currentPost, postExists := relationships["current_post"]
	if !postExists {
		t.Fatal("Was expecting the data.relationships.current_post key/value to have NOT been omitted")
	}
	currentPostMap, ok := currentPost.(map[string]interface{})
	if !ok {
		t.Fatal("data.relationships.current_post was not a map")
	}
	currentPostData, ok := currentPostMap["data"]
	if !ok {
		t.Fatal("Was expecting the data.relationships.current_post.data key/value to have been present")
	}
	if currentPostData != nil {
		t.Fatal("Was expecting the data.relationships.current_post.data value to have been nil/null")
	}
}

func TestWithOmitsEmptyAnnotationOnRelation(t *testing.T) {
	type BlogOptionalPosts struct {
		ID          int     `jsonapi:"primary,blogs"`
		Title       string  `jsonapi:"attr,title"`
		Posts       []*Post `jsonapi:"relation,posts,omitempty"`
		CurrentPost *Post   `jsonapi:"relation,current_post,omitempty"`
	}

	blog := &BlogOptionalPosts{ID: 999}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, blog); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	payload := jsonData["data"].(map[string]interface{})

	// Verify relationship was NOT set
	if val, exists := payload["relationships"]; exists {
		t.Fatalf("Was expecting the data.relationships key/value to have been empty - it was not and had a value of %v", val)
	}
}

func TestWithOmitsEmptyAnnotationOnRelation_MixedData(t *testing.T) {
	type BlogOptionalPosts struct {
		ID          int     `jsonapi:"primary,blogs"`
		Title       string  `jsonapi:"attr,title"`
		Posts       []*Post `jsonapi:"relation,posts,omitempty"`
		CurrentPost *Post   `jsonapi:"relation,current_post,omitempty"`
	}

	blog := &BlogOptionalPosts{
		ID: 999,
		CurrentPost: &Post{
			ID: 123,
		},
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, blog); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	payload := jsonData["data"].(map[string]interface{})

	// Verify relationship was set
	if _, exists := payload["relationships"]; !exists {
		t.Fatal("Was expecting the data.relationships key/value to have NOT been empty")
	}

	relationships := payload["relationships"].(map[string]interface{})

	// Verify the relationship was not omitted, and is not null
	if val, exists := relationships["current_post"]; !exists {
		t.Fatal("Was expecting the data.relationships.current_post key/value to have NOT been omitted")
	} else if val.(map[string]interface{})["data"] == nil {
		t.Fatal("Was expecting the data.relationships.current_post value to have NOT been nil/null")
	}
}

func TestMarshalIDPtr(t *testing.T) {
	id, make, model := "123e4567-e89b-12d3-a456-426655440000", "Ford", "Mustang"
	car := &Car{
		ID:    &id,
		Make:  &make,
		Model: &model,
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, car); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	data := jsonData["data"].(map[string]interface{})
	// attributes := data["attributes"].(map[string]interface{})

	// Verify that the ID was sent
	val, exists := data["id"]
	if !exists {
		t.Fatal("Was expecting the data.id member to exist")
	}
	if val != id {
		t.Fatalf("Was expecting the data.id member to be `%s`, got `%s`", id, val)
	}
}

func TestMarshalOnePayload_omitIDString(t *testing.T) {
	type Foo struct {
		ID    string `jsonapi:"primary,foo"`
		Title string `jsonapi:"attr,title"`
	}

	foo := &Foo{Title: "Foo"}
	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, foo); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	payload := jsonData["data"].(map[string]interface{})

	// Verify that empty ID of type string gets omitted. See:
	// https://github.com/google/jsonapi/issues/83#issuecomment-285611425
	_, ok := payload["id"]
	if ok {
		t.Fatal("Was expecting the data.id member to be omitted")
	}
}

func TestMarshall_invalidIDType(t *testing.T) {
	type badIDStruct struct {
		ID *bool `jsonapi:"primary,cars"`
	}
	id := true
	o := &badIDStruct{ID: &id}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, o); err != ErrBadJSONAPIID {
		t.Fatalf(
			"Was expecting a `%s` error, got `%s`", ErrBadJSONAPIID, err,
		)
	}
}

func TestOmitsEmptyAnnotation(t *testing.T) {
	book := &Book{
		Author:      "aren55555",
		PublishedAt: time.Now().AddDate(0, -1, 0),
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, book); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	attributes := jsonData["data"].(map[string]interface{})["attributes"].(map[string]interface{})

	// Verify that the specifically omitted field were omitted
	if val, exists := attributes["title"]; exists {
		t.Fatalf("Was expecting the data.attributes.title key/value to have been omitted - it was not and had a value of %v", val)
	}
	if val, exists := attributes["pages"]; exists {
		t.Fatalf("Was expecting the data.attributes.pages key/value to have been omitted - it was not and had a value of %v", val)
	}

	// Verify the implicity omitted fields were omitted
	if val, exists := attributes["PublishedAt"]; exists {
		t.Fatalf("Was expecting the data.attributes.PublishedAt key/value to have been implicity omitted - it was not and had a value of %v", val)
	}

	// Verify the unset fields were not omitted
	if _, exists := attributes["isbn"]; !exists {
		t.Fatal("Was expecting the data.attributes.isbn key/value to have NOT been omitted")
	}
}

func TestHasPrimaryAnnotation(t *testing.T) {
	testModel := &Blog{
		ID:        5,
		Title:     "Title 1",
		CreatedAt: time.Now(),
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)

	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	data := resp.Data

	if data.Type != "blogs" {
		t.Fatalf("type should have been blogs, got %s", data.Type)
	}

	if data.ID != "5" {
		t.Fatalf("ID not transfered")
	}
}

func TestSupportsAttributes(t *testing.T) {
	testModel := &Blog{
		ID:        5,
		Title:     "Title 1",
		CreatedAt: time.Now(),
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	data := resp.Data

	if data.Attributes == nil {
		t.Fatalf("Expected attributes")
	}

	if data.Attributes["title"] != "Title 1" {
		t.Fatalf("Attributes hash not populated using tags correctly")
	}
}

func TestOmitsZeroTimes(t *testing.T) {
	testModel := &Blog{
		ID:        5,
		Title:     "Title 1",
		CreatedAt: time.Time{},
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	data := resp.Data

	if data.Attributes == nil {
		t.Fatalf("Expected attributes")
	}

	if data.Attributes["created_at"] != nil {
		t.Fatalf("Created at was serialized even though it was a zero Time")
	}
}

func TestMarshalISO8601Time(t *testing.T) {
	testModel := &Timestamp{
		ID:   5,
		Time: time.Date(2016, 8, 17, 8, 27, 12, 23849, time.UTC),
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	data := resp.Data

	if data.Attributes == nil {
		t.Fatalf("Expected attributes")
	}

	if data.Attributes["timestamp"] != "2016-08-17T08:27:12Z" {
		t.Fatal("Timestamp was not serialised into ISO8601 correctly")
	}
}

func TestMarshalISO8601TimePointer(t *testing.T) {
	tm := time.Date(2016, 8, 17, 8, 27, 12, 23849, time.UTC)
	testModel := &Timestamp{
		ID:   5,
		Next: &tm,
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	data := resp.Data

	if data.Attributes == nil {
		t.Fatalf("Expected attributes")
	}

	if data.Attributes["next"] != "2016-08-17T08:27:12Z" {
		t.Fatal("Next was not serialised into ISO8601 correctly")
	}
}

func TestSupportsLinkable(t *testing.T) {
	testModel := &Blog{
		ID:        5,
		Title:     "Title 1",
		CreatedAt: time.Now(),
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	data := resp.Data

	if data.Links == nil {
		t.Fatal("Expected data.links")
	}
	links := *data.Links

	self, hasSelf := links["self"]
	if !hasSelf {
		t.Fatal("Expected 'self' link to be present")
	}
	if _, isString := self.(string); !isString {
		t.Fatal("Expected 'self' to contain a string")
	}

	comments, hasComments := links["comments"]
	if !hasComments {
		t.Fatal("expect 'comments' to be present")
	}
	commentsMap, isMap := comments.(map[string]interface{})
	if !isMap {
		t.Fatal("Expected 'comments' to contain a map")
	}

	commentsHref, hasHref := commentsMap["href"]
	if !hasHref {
		t.Fatal("Expect 'comments' to contain an 'href' key/value")
	}
	if _, isString := commentsHref.(string); !isString {
		t.Fatal("Expected 'href' to contain a string")
	}

	commentsMeta, hasMeta := commentsMap["meta"]
	if !hasMeta {
		t.Fatal("Expect 'comments' to contain a 'meta' key/value")
	}
	commentsMetaMap, isMap := commentsMeta.(map[string]interface{})
	if !isMap {
		t.Fatal("Expected 'comments' to contain a map")
	}

	commentsMetaObject := Meta(commentsMetaMap)
	countsMap, isMap := commentsMetaObject["counts"].(map[string]interface{})
	if !isMap {
		t.Fatal("Expected 'counts' to contain a map")
	}
	for k, v := range countsMap {
		if _, isNum := v.(float64); !isNum {
			t.Fatalf("Exepected value at '%s' to be a numeric (float64)", k)
		}
	}
}

func TestInvalidLinkable(t *testing.T) {
	testModel := &BadComment{
		ID:   5,
		Body: "Hello World",
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err == nil {
		t.Fatal("Was expecting an error")
	}
}

func TestSupportsMetable(t *testing.T) {
	testModel := &Blog{
		ID:        5,
		Title:     "Title 1",
		CreatedAt: time.Now(),
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	data := resp.Data
	if data.Meta == nil {
		t.Fatalf("Expected data.meta")
	}

	meta := Meta(*data.Meta)
	if e, a := "extra details regarding the blog", meta["detail"]; e != a {
		t.Fatalf("Was expecting meta.detail to be %q, got %q", e, a)
	}
}

func TestRelations(t *testing.T) {
	testModel := testBlog()

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	relations := resp.Data.Relationships

	if relations == nil {
		t.Fatalf("Relationships were not materialized")
	}

	if relations["posts"] == nil {
		t.Fatalf("Posts relationship was not materialized")
	} else {
		if relations["posts"].(map[string]interface{})["links"] == nil {
			t.Fatalf("Posts relationship links were not materialized")
		}
		if relations["posts"].(map[string]interface{})["meta"] == nil {
			t.Fatalf("Posts relationship meta were not materialized")
		}
	}

	if relations["current_post"] == nil {
		t.Fatalf("Current post relationship was not materialized")
	} else {
		if relations["current_post"].(map[string]interface{})["links"] == nil {
			t.Fatalf("Current post relationship links were not materialized")
		}
		if relations["current_post"].(map[string]interface{})["meta"] == nil {
			t.Fatalf("Current post relationship meta were not materialized")
		}
	}

	if len(relations["posts"].(map[string]interface{})["data"].([]interface{})) != 2 {
		t.Fatalf("Did not materialize two posts")
	}
}

func TestNoRelations(t *testing.T) {
	testModel := &Blog{ID: 1, Title: "Title 1", CreatedAt: time.Now()}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, testModel); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	if resp.Included != nil {
		t.Fatalf("Encoding json response did not omit included")
	}
}

func TestMarshalPayloadWithoutIncluded(t *testing.T) {
	data := &Post{
		ID:       1,
		BlogID:   2,
		ClientID: "123e4567-e89b-12d3-a456-426655440000",
		Title:    "Foo",
		Body:     "Bar",
		Comments: []*Comment{
			&Comment{
				ID:   20,
				Body: "First",
			},
			&Comment{
				ID:   21,
				Body: "Hello World",
			},
		},
		LatestComment: &Comment{
			ID:   22,
			Body: "Cool!",
		},
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayloadWithoutIncluded(out, data); err != nil {
		t.Fatal(err)
	}

	resp := new(OnePayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	if resp.Included != nil {
		t.Fatalf("Encoding json response did not omit included")
	}
}

func TestMarshalPayload_many(t *testing.T) {
	data := []interface{}{
		&Blog{
			ID:        5,
			Title:     "Title 1",
			CreatedAt: time.Now(),
			Posts: []*Post{
				&Post{
					ID:    1,
					Title: "Foo",
					Body:  "Bar",
				},
				&Post{
					ID:    2,
					Title: "Fuubar",
					Body:  "Bas",
				},
			},
			CurrentPost: &Post{
				ID:    1,
				Title: "Foo",
				Body:  "Bar",
			},
		},
		&Blog{
			ID:        6,
			Title:     "Title 2",
			CreatedAt: time.Now(),
			Posts: []*Post{
				&Post{
					ID:    3,
					Title: "Foo",
					Body:  "Bar",
				},
				&Post{
					ID:    4,
					Title: "Fuubar",
					Body:  "Bas",
				},
			},
			CurrentPost: &Post{
				ID:    4,
				Title: "Foo",
				Body:  "Bar",
			},
		},
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, data); err != nil {
		t.Fatal(err)
	}

	resp := new(ManyPayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	d := resp.Data

	if len(d) != 2 {
		t.Fatalf("data should have two elements")
	}
}

func TestMarshalMany_WithSliceOfStructPointers(t *testing.T) {
	var data []*Blog
	for len(data) < 2 {
		data = append(data, testBlog())
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, data); err != nil {
		t.Fatal(err)
	}

	resp := new(ManyPayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	d := resp.Data

	if len(d) != 2 {
		t.Fatalf("data should have two elements")
	}
}

func TestMarshalManyWithoutIncluded(t *testing.T) {
	var data []*Blog
	for len(data) < 2 {
		data = append(data, testBlog())
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayloadWithoutIncluded(out, data); err != nil {
		t.Fatal(err)
	}

	resp := new(ManyPayload)
	if err := json.NewDecoder(out).Decode(resp); err != nil {
		t.Fatal(err)
	}

	d := resp.Data

	if len(d) != 2 {
		t.Fatalf("data should have two elements")
	}

	if resp.Included != nil {
		t.Fatalf("Encoding json response did not omit included")
	}
}

func TestMarshalMany_SliceOfInterfaceAndSliceOfStructsSameJSON(t *testing.T) {
	structs := []*Book{
		&Book{ID: 1, Author: "aren55555", ISBN: "abc"},
		&Book{ID: 2, Author: "shwoodard", ISBN: "xyz"},
	}
	interfaces := []interface{}{}
	for _, s := range structs {
		interfaces = append(interfaces, s)
	}

	// Perform Marshals
	structsOut := new(bytes.Buffer)
	if err := MarshalPayload(structsOut, structs); err != nil {
		t.Fatal(err)
	}
	interfacesOut := new(bytes.Buffer)
	if err := MarshalPayload(interfacesOut, interfaces); err != nil {
		t.Fatal(err)
	}

	// Generic JSON Unmarshal
	structsData, interfacesData :=
		make(map[string]interface{}), make(map[string]interface{})
	if err := json.Unmarshal(structsOut.Bytes(), &structsData); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(interfacesOut.Bytes(), &interfacesData); err != nil {
		t.Fatal(err)
	}

	// Compare Result
	if !reflect.DeepEqual(structsData, interfacesData) {
		t.Fatal("Was expecting the JSON API generated to be the same")
	}
}

func TestMarshal_InvalidIntefaceArgument(t *testing.T) {
	out := new(bytes.Buffer)
	if err := MarshalPayload(out, true); err != ErrUnexpectedType {
		t.Fatal("Was expecting an error")
	}
	if err := MarshalPayload(out, 25); err != ErrUnexpectedType {
		t.Fatal("Was expecting an error")
	}
	if err := MarshalPayload(out, Book{}); err != ErrUnexpectedType {
		t.Fatal("Was expecting an error")
	}
}

func TestMergeNode(t *testing.T) {
	parent := &Node{
		Type:       "Good",
		ID:         "99",
		Attributes: map[string]interface{}{"fizz": "buzz"},
	}

	child := &Node{
		Type:       "Better",
		ClientID:   "1111",
		Attributes: map[string]interface{}{"timbuk": 2},
	}

	expected := &Node{
		Type:       "Better",
		ID:         "99",
		ClientID:   "1111",
		Attributes: map[string]interface{}{"fizz": "buzz", "timbuk": 2},
	}

	parent.merge(child)

	if !reflect.DeepEqual(expected, parent) {
		t.Errorf("Got %+v Expected %+v", parent, expected)
	}
}

// TestEmbeddedUnmarshalOrder tests the behavior of the marshaler/unmarshaler of embedded structs
// when a struct has an embedded struct w/ competing attributes, the top-level attributes take precedence
// it compares the behavior against the standard json package
func TestEmbeddedUnmarshalOrder(t *testing.T) {
	type Bar struct {
		Name int `jsonapi:"attr,Name"`
	}

	type Foo struct {
		Bar
		ID   string `jsonapi:"primary,foos"`
		Name string `jsonapi:"attr,Name"`
	}

	f := &Foo{
		ID:   "1",
		Name: "foo",
		Bar: Bar{
			Name: 5,
		},
	}

	// marshal f (Foo) using jsonapi marshaler
	jsonAPIData := bytes.NewBuffer(nil)
	if err := MarshalPayload(jsonAPIData, f); err != nil {
		t.Fatal(err)
	}

	// marshal f (Foo) using json marshaler
	jsonData, err := json.Marshal(f)

	// convert bytes to map[string]interface{} so that we can do a semantic JSON comparison
	var jsonAPIVal, jsonVal map[string]interface{}
	if err := json.Unmarshal(jsonAPIData.Bytes(), &jsonAPIVal); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(jsonData, &jsonVal); err != nil {
		t.Fatal(err)
	}

	// get to the jsonapi attribute map
	jAttrMap := jsonAPIVal["data"].(map[string]interface{})["attributes"].(map[string]interface{})

	// compare
	if !reflect.DeepEqual(jAttrMap["Name"], jsonVal["Name"]) {
		t.Errorf("Got\n%s\nExpected\n%s\n", jAttrMap["Name"], jsonVal["Name"])
	}
}

// TestEmbeddedMarshalOrder tests the behavior of the marshaler/unmarshaler of embedded structs
// when a struct has an embedded struct w/ competing attributes, the top-level attributes take precedence
// it compares the behavior against the standard json package
func TestEmbeddedMarshalOrder(t *testing.T) {
	type Bar struct {
		Name int `jsonapi:"attr,Name"`
	}

	type Foo struct {
		Bar
		ID   string `jsonapi:"primary,foos"`
		Name string `jsonapi:"attr,Name"`
	}

	// get a jsonapi payload w/ Name attribute of an int type
	payloadWithInt, err := json.Marshal(&OnePayload{
		Data: &Node{
			Type: "foos",
			ID:   "1",
			Attributes: map[string]interface{}{
				"Name": 5,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// get a jsonapi payload w/ Name attribute of an string type
	payloadWithString, err := json.Marshal(&OnePayload{
		Data: &Node{
			Type: "foos",
			ID:   "1",
			Attributes: map[string]interface{}{
				"Name": "foo",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// unmarshal payloadWithInt to f (Foo) using jsonapi unmarshaler; expecting an error
	f := &Foo{}
	if err := UnmarshalPayload(bytes.NewReader(payloadWithInt), f); err == nil {
		t.Errorf("expected an error: int value of 5 should attempt to map to Foo.Name (string) and error")
	}

	// unmarshal payloadWithString to f (Foo) using jsonapi unmarshaler; expecting no error
	f = &Foo{}
	if err := UnmarshalPayload(bytes.NewReader(payloadWithString), f); err != nil {
		t.Error(err)
	}
	if f.Name != "foo" {
		t.Errorf("Got\n%s\nExpected\n%s\n", "foo", f.Name)
	}

	// get a json payload w/ Name attribute of an int type
	bWithInt, err := json.Marshal(map[string]interface{}{
		"Name": 5,
	})
	if err != nil {
		t.Fatal(err)
	}

	// get a json payload w/ Name attribute of an string type
	bWithString, err := json.Marshal(map[string]interface{}{
		"Name": "foo",
	})
	if err != nil {
		t.Fatal(err)
	}

	// unmarshal bWithInt to f (Foo) using json unmarshaler; expecting an error
	f = &Foo{}
	if err := json.Unmarshal(bWithInt, f); err == nil {
		t.Errorf("expected an error: int value of 5 should attempt to map to Foo.Name (string) and error")
	}
	// unmarshal bWithString to f (Foo) using json unmarshaler; expecting no error
	f = &Foo{}
	if err := json.Unmarshal(bWithString, f); err != nil {
		t.Error(err)
	}
	if f.Name != "foo" {
		t.Errorf("Got\n%s\nExpected\n%s\n", "foo", f.Name)
	}
}

func TestMarshalUnmarshalCompositeStruct(t *testing.T) {
	type Thing struct {
		ID   int    `jsonapi:"primary,things"`
		Fizz string `jsonapi:"attr,fizz,omitempty"`
		Buzz int    `jsonapi:"attr,buzz,omitempty"`
	}

	type Model struct {
		*Thing `jsonapi:"extends,models"`
		Foo    string `jsonapi:"attr,foo"`
		Bar    string `jsonapi:"attr,bar"`
		Bat    string `jsonapi:"attr,bat"`
		Buzz   int    `jsonapi:"attr,buzz,omitempty"` // overrides Thing.Buzz
	}

	type test struct {
		name          string
		payload       *OnePayload
		dst, expected interface{}
	}

	scenarios := []test{}

	scenarios = append(scenarios, test{
		name: "Model embeds Thing, models have no annotation overlaps",
		dst:  &Model{Thing: &Thing{}},
		payload: &OnePayload{
			Data: &Node{
				Type: "models",
				ID:   "1",
				Attributes: map[string]interface{}{
					"bar":  "barry",
					"bat":  "batty",
					"buzz": 99,
					"fizz": "fizzy",
					"foo":  "fooey",
				},
			},
		},
		expected: &Model{
			Foo: "fooey",
			Bar: "barry",
			Bat: "batty",
			Thing: &Thing{
				ID:   1,
				Fizz: "fizzy",
				Buzz: 99,
			},
		},
	})

	{
		scenarios = append(scenarios, test{
			name: "Model embeds Thing, overlap Buzz attribute",
			dst:  &Model{Thing: &Thing{}},
			payload: &OnePayload{
				Data: &Node{
					Type: "models",
					ID:   "1",
					Attributes: map[string]interface{}{
						"bar":  "barry",
						"bat":  "batty",
						"buzz": 99,
						"fizz": "fizzy",
						"foo":  "fooey",
					},
				},
			},
			expected: &Model{
				Foo:  "fooey",
				Bar:  "barry",
				Bat:  "batty",
				Buzz: 99,
				Thing: &Thing{
					ID:   1,
					Fizz: "fizzy",
				},
			},
		})
	}
	{
		scenarios = append(scenarios, test{
			name: "Model embeds pointer of Thing; Thing is initialized in advance",
			dst:  &Model{Thing: &Thing{}},
			payload: &OnePayload{
				Data: &Node{
					Type: "models",
					ID:   "1",
					Attributes: map[string]interface{}{
						"bar":  "barry",
						"bat":  "batty",
						"foo":  "fooey",
						"buzz": 99,
						"fizz": "fizzy",
					},
				},
			},
			expected: &Model{
				Thing: &Thing{
					ID:   1,
					Fizz: "fizzy",
					Buzz: 99,
				},
				Foo: "fooey",
				Bar: "barry",
				Bat: "batty",
			},
		})
	}
	{
		scenarios = append(scenarios, test{
			name: "Model embeds pointer of Thing; Thing is initialized w/ Unmarshal",
			dst:  &Model{Thing: &Thing{}},
			payload: &OnePayload{
				Data: &Node{
					Type: "models",
					ID:   "1",
					Attributes: map[string]interface{}{
						"bar":  "barry",
						"bat":  "batty",
						"foo":  "fooey",
						"buzz": 99,
						"fizz": "fizzy",
					},
				},
			},
			expected: &Model{
				Thing: &Thing{
					ID:   1,
					Fizz: "fizzy",
					Buzz: 99,
				},
				Foo: "fooey",
				Bar: "barry",
				Bat: "batty",
			},
		})
	}
	for _, scenario := range scenarios {
		t.Logf("running scenario: %s\n", scenario.name)

		// get the expected model and marshal to jsonapi
		buf := bytes.NewBuffer(nil)
		if err := MarshalPayload(buf, scenario.expected); err != nil {
			t.Fatal(err)
		}

		// get the node model representation and marshal to jsonapi
		payload, err := json.Marshal(scenario.payload)
		if err != nil {
			t.Fatal(err)
		}

		// assert that we're starting w/ the same payload
		isJSONEqual, err := isJSONEqual(payload, buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if !isJSONEqual {
			t.Errorf("Got\n%s\nExpected\n%s\n", buf.Bytes(), payload)
		}

		// run jsonapi unmarshal
		if err := UnmarshalPayload(bytes.NewReader(payload), scenario.dst); err != nil {
			t.Fatal(err)
		}

		if scenario.expected.(*Model).ID == scenario.dst.(*Model).ID {
			t.Errorf("Expected matching ID's but were \n%#v\nAnd\n%#v\n", scenario.expected.(*Model).ID, scenario.dst.(*Model).ID)
		}

		// assert decoded and expected models are equal
		if scenario.expected == scenario.dst {
			t.Errorf("Got\n%#v\nExpected\n%#v\n", scenario.dst, scenario.expected)
		}
	}
}

func TestExtendsWithRelation_MixedData(t *testing.T) {
	type Thing struct {
		ID   int    `jsonapi:"primary,things"`
		Fizz string `jsonapi:"attr,fizz,omitempty"`
		Buzz int    `jsonapi:"attr,buzz,omitempty"`
	}

	type Relation struct {
		ID int `jsonapi:"primary,relations"`
	}

	type Model struct {
		*Thing `jsonapi:"extends,models"`
		Foo    string      `jsonapi:"attr,foo"`
		Bar    string      `jsonapi:"attr,bar"`
		Bat    string      `jsonapi:"attr,bat"`
		Buzz   int         `jsonapi:"attr,buzz,omitempty"` // overrides Thing.Buzz
		Rels   []*Relation `jsonapi:"relation,relations,omitempty"`
	}

	model := &Model{
		Thing: &Thing{
			ID:   1,
			Fizz: "fizzy",
			Buzz: 99,
		},
		Foo:  "fooey",
		Bar:  "barry",
		Bat:  "batty",
		Rels: []*Relation{{ID: 1}, {ID: 2}},
	}

	out := bytes.NewBuffer(nil)
	if err := MarshalPayload(out, model); err != nil {
		t.Fatal(err)
	}

	var jsonData map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &jsonData); err != nil {
		t.Fatal(err)
	}
	payload := jsonData["data"].(map[string]interface{})

	// Verify relationship was set
	if _, exists := payload["relationships"]; !exists {
		t.Fatal("Was expecting the data.relationships key/value to have NOT been empty")
	}

	relationships := payload["relationships"].(map[string]interface{})

	// Verify the relationship was not omitted, and is not null
	val, exists := relationships["relations"]
	if !exists {
		t.Fatal("Was expecting the data.relationships.relations key/value to have NOT been omitted")
	}

	relData := val.(map[string]interface{})

	rels, exists := relData["data"]
	if !exists {
		t.Fatal("Was expecting the data.relationships.relations.data key/value to have NOT been omitted")
	}

	l := len(rels.([]interface{}))
	if l != 2 {
		t.Fatal("Was expecting 2 relations but there were %d", l)
	}
	fmt.Println(string(out.Bytes()))

	m := Model{Thing: new(Thing), Rels: make([]*Relation, 0)}
	if err := UnmarshalPayload(out, m); err != nil {
		t.Fatal(err)
	}

}

func TestMarshalUnmarshalCompositeStruct_Errors(t *testing.T) {
	type Thing struct {
		ID   string `jsonapi:"primary,things"`
		Fizz string `jsonapi:"attr,fizz,omitempty"`
		Buzz int    `jsonapi:"attr,buzz,omitempty"`
	}

	type Model struct {
		*Thing `jsonapi:"embedded,models"`
		Foo    string `jsonapi:"attr,foo"`
		Bar    string `jsonapi:"attr,bar"`
		Bat    string `jsonapi:"attr,bat"`
	}

	type test struct {
		name     string
		payload  *OnePayload
		dst      *Model
		expected error
	}

	scenarios := []test{}

	{

		scenarios = append(scenarios, test{
			name: "Model embeds pointer of Thing; *Thing is nil",
			dst:  &Model{},
			payload: &OnePayload{
				Data: &Node{
					Type: "models",
					Attributes: map[string]interface{}{
						"bar": "barry",
						"bat": "batty",
						"foo": "fooey",
					},
				},
			},
			expected: ErrEmbeddedPtrNotSet,
		})
	}

	for _, scenario := range scenarios {
		t.Logf("running scenario: %s\n", scenario.name)

		// get the expected model and marshal to jsonapi
		buf := bytes.NewBuffer(nil)
		if err := MarshalPayload(buf, scenario.dst); err != scenario.expected {
			t.Errorf("Dst\n%#v\nGot\n%#v\nExpected\n%#v\n", scenario.dst.ID, err, scenario.expected)
		}
	}
}

func testBlog() *Blog {
	return &Blog{
		ID:        5,
		Title:     "Title 1",
		CreatedAt: time.Now(),
		Posts: []*Post{
			&Post{
				ID:    1,
				Title: "Foo",
				Body:  "Bar",
				Comments: []*Comment{
					&Comment{
						ID:   1,
						Body: "foo",
					},
					&Comment{
						ID:   2,
						Body: "bar",
					},
				},
				LatestComment: &Comment{
					ID:   1,
					Body: "foo",
				},
			},
			&Post{
				ID:    2,
				Title: "Fuubar",
				Body:  "Bas",
				Comments: []*Comment{
					&Comment{
						ID:   1,
						Body: "foo",
					},
					&Comment{
						ID:   3,
						Body: "bas",
					},
				},
				LatestComment: &Comment{
					ID:   1,
					Body: "foo",
				},
			},
		},
		CurrentPost: &Post{
			ID:    1,
			Title: "Foo",
			Body:  "Bar",
			Comments: []*Comment{
				&Comment{
					ID:   1,
					Body: "foo",
				},
				&Comment{
					ID:   2,
					Body: "bar",
				},
			},
			LatestComment: &Comment{
				ID:   1,
				Body: "foo",
			},
		},
	}
}

func isJSONEqual(b1, b2 []byte) (bool, error) {
	var i1, i2 interface{}
	var result bool
	var err error
	if err = json.Unmarshal(b1, &i1); err != nil {
		return result, err
	}
	if err = json.Unmarshal(b2, &i2); err != nil {
		return result, err
	}
	result = reflect.DeepEqual(i1, i2)
	return result, err
}
