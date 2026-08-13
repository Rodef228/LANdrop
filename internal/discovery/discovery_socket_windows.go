//go:build windows

package discovery

import "syscall"

// setSocketOptions применяет настройки сокета для прослушивания UDP.
// На Windows дескриптор сокета — syscall.Handle, а значения опций
// отличаются от Unix (0x0004 = SO_REUSEADDR, 0x0020 = SO_BROADCAST).
func setSocketOptions(fd uintptr) {
	// SO_REUSEADDR — разрешить повторное использование адреса.
	_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, 0x0004, 1)
	// SO_BROADCAST — разрешить широковещательные (broadcast) пакеты.
	_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, 0x0020, 1)
}
