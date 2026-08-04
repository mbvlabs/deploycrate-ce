package models

import "errors"

type ResourceTypeEnum string

const (
	ResourceTypeDatabase ResourceTypeEnum = "database"
	ResourceTypeCache    ResourceTypeEnum = "cache"
	ResourceTypeService  ResourceTypeEnum = "service"
)

func (resourceType ResourceTypeEnum) IsValid() bool {
	return resourceType == ResourceTypeDatabase || resourceType == ResourceTypeCache || resourceType == ResourceTypeService
}

func (resourceType ResourceTypeEnum) String() string {
	return string(resourceType)
}

func ParseResourceTypeEnum(value string) (ResourceTypeEnum, error) {
	resourceType := ResourceTypeEnum(value)
	if !resourceType.IsValid() {
		return "", errors.New("invalid Resource type")
	}
	return resourceType, nil
}
