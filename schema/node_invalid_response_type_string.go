// Code generated by "enumer --values --type=NodeInvalidResponseType --linecomment --output node_invalid_response_type_string.go --json --yaml --sql"; DO NOT EDIT.

package schema

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

const _NodeInvalidResponseTypeName = "inconsistenterror"

var _NodeInvalidResponseTypeIndex = [...]uint8{0, 12, 17}

const _NodeInvalidResponseTypeLowerName = "inconsistenterror"

func (i NodeInvalidResponseType) String() string {
	if i < 0 || i >= NodeInvalidResponseType(len(_NodeInvalidResponseTypeIndex)-1) {
		return fmt.Sprintf("NodeInvalidResponseType(%d)", i)
	}
	return _NodeInvalidResponseTypeName[_NodeInvalidResponseTypeIndex[i]:_NodeInvalidResponseTypeIndex[i+1]]
}

func (NodeInvalidResponseType) Values() []string {
	return NodeInvalidResponseTypeStrings()
}

// An "invalid array index" compiler error signifies that the constant values have changed.
// Re-run the stringer command to generate them again.
func _NodeInvalidResponseTypeNoOp() {
	var x [1]struct{}
	_ = x[NodeInvalidResponseTypeInconsistent-(0)]
	_ = x[NodeInvalidResponseTypeError-(1)]
}

var _NodeInvalidResponseTypeValues = []NodeInvalidResponseType{NodeInvalidResponseTypeInconsistent, NodeInvalidResponseTypeError}

var _NodeInvalidResponseTypeNameToValueMap = map[string]NodeInvalidResponseType{
	_NodeInvalidResponseTypeName[0:12]:       NodeInvalidResponseTypeInconsistent,
	_NodeInvalidResponseTypeLowerName[0:12]:  NodeInvalidResponseTypeInconsistent,
	_NodeInvalidResponseTypeName[12:17]:      NodeInvalidResponseTypeError,
	_NodeInvalidResponseTypeLowerName[12:17]: NodeInvalidResponseTypeError,
}

var _NodeInvalidResponseTypeNames = []string{
	_NodeInvalidResponseTypeName[0:12],
	_NodeInvalidResponseTypeName[12:17],
}

// NodeInvalidResponseTypeString retrieves an enum value from the enum constants string name.
// Throws an error if the param is not part of the enum.
func NodeInvalidResponseTypeString(s string) (NodeInvalidResponseType, error) {
	if val, ok := _NodeInvalidResponseTypeNameToValueMap[s]; ok {
		return val, nil
	}

	if val, ok := _NodeInvalidResponseTypeNameToValueMap[strings.ToLower(s)]; ok {
		return val, nil
	}
	return 0, fmt.Errorf("%s does not belong to NodeInvalidResponseType values", s)
}

// NodeInvalidResponseTypeValues returns all values of the enum
func NodeInvalidResponseTypeValues() []NodeInvalidResponseType {
	return _NodeInvalidResponseTypeValues
}

// NodeInvalidResponseTypeStrings returns a slice of all String values of the enum
func NodeInvalidResponseTypeStrings() []string {
	strs := make([]string, len(_NodeInvalidResponseTypeNames))
	copy(strs, _NodeInvalidResponseTypeNames)
	return strs
}

// IsANodeInvalidResponseType returns "true" if the value is listed in the enum definition. "false" otherwise
func (i NodeInvalidResponseType) IsANodeInvalidResponseType() bool {
	for _, v := range _NodeInvalidResponseTypeValues {
		if i == v {
			return true
		}
	}
	return false
}

// MarshalJSON implements the json.Marshaler interface for NodeInvalidResponseType
func (i NodeInvalidResponseType) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.String())
}

// UnmarshalJSON implements the json.Unmarshaler interface for NodeInvalidResponseType
func (i *NodeInvalidResponseType) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("NodeInvalidResponseType should be a string, got %s", data)
	}

	var err error
	*i, err = NodeInvalidResponseTypeString(s)
	return err
}

// MarshalYAML implements a YAML Marshaler for NodeInvalidResponseType
func (i NodeInvalidResponseType) MarshalYAML() (interface{}, error) {
	return i.String(), nil
}

// UnmarshalYAML implements a YAML Unmarshaler for NodeInvalidResponseType
func (i *NodeInvalidResponseType) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	var err error
	*i, err = NodeInvalidResponseTypeString(s)
	return err
}

func (i NodeInvalidResponseType) Value() (driver.Value, error) {
	return i.String(), nil
}

func (i *NodeInvalidResponseType) Scan(value interface{}) error {
	if value == nil {
		return nil
	}

	var str string
	switch v := value.(type) {
	case []byte:
		str = string(v)
	case string:
		str = v
	case fmt.Stringer:
		str = v.String()
	default:
		return fmt.Errorf("invalid value of NodeInvalidResponseType: %[1]T(%[1]v)", value)
	}

	val, err := NodeInvalidResponseTypeString(str)
	if err != nil {
		return err
	}

	*i = val
	return nil
}
