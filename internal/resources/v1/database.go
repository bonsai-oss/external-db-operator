package v1

import (
	"bytes"
	"encoding/json"
	"regexp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Database struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec DatabaseSpec `json:"spec,omitempty"`
}

type DatabaseSpec struct {
	Database string `json:"database,omitempty"`
}

func (d *Database) AssembleDatabaseName() string {
	return removeIllegalDatabaseCharacters(d.Namespace + "_" + d.Name)
}

func removeIllegalDatabaseCharacters(input string) string {
	return regexp.MustCompile("[.-]+").ReplaceAllString(input, "_")
}

func FromUnstructured(data any) (*Database, error) {
	buf := bytes.NewBuffer(nil)
	databaseResourceData := &Database{}
	if encodeError := json.NewEncoder(buf).Encode(data); encodeError != nil {
		return nil, encodeError
	}
	decodeError := json.NewDecoder(buf).Decode(databaseResourceData)

	return databaseResourceData, decodeError
}
