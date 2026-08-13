//go:build !windows

package discovery

import "syscall"

// setSocketOptions применяет настройки сокета для прослушивания UDP.
// На Unix-подобных системах (Linux, macOS, BSD) используется int(fd)
// и константы из пакета syscall.
func setSocketOptions(fd uintptr) {
	// SO_BROADCAST — разрешить широковещательные (broadcast) пакеты.
	_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	// SO_REUSEADDR — разрешить повторное использование адреса.
	_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
