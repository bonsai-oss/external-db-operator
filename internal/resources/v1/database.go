package v1

import (
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

func (d *Database) AssembleKubernetesName() string {
	return d.Name
}

func removeIllegalDatabaseCharacters(input string) string {
	return regexp.MustCompile("[.-]+").ReplaceAllString(input, "_")
}
