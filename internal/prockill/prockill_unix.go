//go:build !windows

package prockill

// 待接入：Unix 上沿 pgid 或父子关系递归发 SIGTERM，超时后补 SIGKILL。
// M1 只面向 Windows，这里直接返回不支持，调用方会退化成「只启动不重启」。
func tree(root int) ([]Killed, error) {
	return nil, ErrUnsupported
}

func kill(root int) ([]Killed, error) {
	return nil, ErrUnsupported
}
