package model

type LogAttrKey string
type LogAttrValue any
type LogAttr struct {
	Key   LogAttrKey
	Value LogAttrValue
}
