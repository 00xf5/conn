//go:build !windows

package privops

func ServePipe(stop <-chan struct{}) {
	<-stop
}

func Call(Request) Response {
	return Response{OK: false, Error: "privileged ops require Windows"}
}
