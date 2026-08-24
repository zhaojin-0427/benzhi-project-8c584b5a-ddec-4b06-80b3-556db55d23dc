package domain

import "errors"

var (
	ErrValidation          = errors.New("业务数据校验失败")
	ErrInvalidTransition   = errors.New("当前状态不允许执行该操作")
	ErrVersionConflict     = errors.New("档案版本已变化")
	ErrNotFound            = errors.New("档案不存在")
	ErrIdempotencyConflict = errors.New("幂等键已用于不同请求")
	ErrDuplicateAccession  = errors.New("馆藏编号已存在")
	ErrForbidden           = errors.New("当前角色无权执行该操作")
	ErrIntegrity           = errors.New("持久化数据完整性校验失败")
)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ValidationError struct {
	Fields []FieldError `json:"fields"`
}

func (e *ValidationError) Error() string { return ErrValidation.Error() }
func (e *ValidationError) Unwrap() error { return ErrValidation }

func invalid(field, message string) error {
	return &ValidationError{Fields: []FieldError{{Field: field, Message: message}}}
}
