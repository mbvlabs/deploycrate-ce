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

type CaddyRouteStateEnum string

const (
	CaddyRoutePending        CaddyRouteStateEnum = "pending"
	CaddyRouteApplied        CaddyRouteStateEnum = "applied"
	CaddyRouteFailed         CaddyRouteStateEnum = "failed"
	CaddyRouteRemovalPending CaddyRouteStateEnum = "removal_pending"
	CaddyRouteRemoved        CaddyRouteStateEnum = "removed"
)

func (state CaddyRouteStateEnum) IsValid() bool {
	switch state {
	case CaddyRoutePending, CaddyRouteApplied, CaddyRouteFailed, CaddyRouteRemovalPending, CaddyRouteRemoved:
		return true
	default:
		return false
	}
}

func (state CaddyRouteStateEnum) String() string {
	return string(state)
}

func ParseCaddyRouteStateEnum(value string) (CaddyRouteStateEnum, error) {
	state := CaddyRouteStateEnum(value)
	if !state.IsValid() {
		return "", errors.New("invalid Caddy route state")
	}
	return state, nil
}
