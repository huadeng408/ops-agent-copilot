package app

var (
	approverRoles = map[string]struct{}{
		"admin":    {},
		"approver": {},
	}
	writeRoles = map[string]struct{}{
		"admin":    {},
		"approver": {},
		"ops":      {},
		"support":  {},
		"manager":  {},
	}
)

func EnsureCanSubmitWrite(user User) error {
	if _, ok := writeRoles[user.Role]; ok {
		return nil
	}
	return NewPermissionDenied("当前用户没有提交写操作申请的权限")
}

func EnsureCanApprove(user User) error {
	if _, ok := approverRoles[user.Role]; ok {
		return nil
	}
	return NewPermissionDenied("当前用户没有审批权限")
}
