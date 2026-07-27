package models

import "errors"

type ResourceManagementModeEnum string

const (
	ResourceManagementManaged  ResourceManagementModeEnum = "managed"
	ResourceManagementExternal ResourceManagementModeEnum = "external"
)

func (mode ResourceManagementModeEnum) IsValid() bool {
	return mode == ResourceManagementManaged || mode == ResourceManagementExternal
}

func (mode ResourceManagementModeEnum) String() string {
	return string(mode)
}

func ParseResourceManagementModeEnum(value string) (ResourceManagementModeEnum, error) {
	mode := ResourceManagementModeEnum(value)
	if !mode.IsValid() {
		return "", errors.New("invalid Resource management mode")
	}
	return mode, nil
}

type ResourceSharingScopeEnum string

const (
	ResourceSharingEnvironment ResourceSharingScopeEnum = "environment"
	ResourceSharingApplication ResourceSharingScopeEnum = "application"
	ResourceSharingGlobal      ResourceSharingScopeEnum = "global"
)

func (scope ResourceSharingScopeEnum) IsValid() bool {
	return scope == ResourceSharingEnvironment || scope == ResourceSharingApplication || scope == ResourceSharingGlobal
}

func (scope ResourceSharingScopeEnum) String() string {
	return string(scope)
}

func ParseResourceSharingScopeEnum(value string) (ResourceSharingScopeEnum, error) {
	scope := ResourceSharingScopeEnum(value)
	if !scope.IsValid() {
		return "", errors.New("invalid Resource sharing scope")
	}
	return scope, nil
}
