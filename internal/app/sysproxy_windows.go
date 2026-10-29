package app

import (
	"net/url"
	"syscall"
	"unsafe"
)

// platformSystemProxy Internet 选项里的代理（HKCU\…\Internet Settings 的 ProxyEnable / ProxyServer）
func platformSystemProxy() *url.URL {
	path, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Internet Settings`)
	var k syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, path, 0, syscall.KEY_QUERY_VALUE, &k) != nil {
		return nil
	}
	defer syscall.RegCloseKey(k)
	var on uint32
	n := uint32(4)
	name, _ := syscall.UTF16PtrFromString("ProxyEnable")
	if syscall.RegQueryValueEx(k, name, nil, nil, (*byte)(unsafe.Pointer(&on)), &n) != nil || on == 0 {
		return nil
	}
	buf := make([]uint16, 1024)
	n = uint32(len(buf) * 2)
	name, _ = syscall.UTF16PtrFromString("ProxyServer")
	if syscall.RegQueryValueEx(k, name, nil, nil, (*byte)(unsafe.Pointer(&buf[0])), &n) != nil {
		return nil
	}
	return parseProxyServer(syscall.UTF16ToString(buf))
}
