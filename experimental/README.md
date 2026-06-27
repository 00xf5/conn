# Experimental tools

Not used by the stable connect-agent build. For hardware pipeline research only.

## Native in-process encoder test

Requires `-tags experimental` (links NVENC/QSV bridge — crashes on some Intel UHD GPUs):

```powershell
go build -tags experimental -o captureenc-test.exe ./experimental/captureenc-test
.\captureenc-test.exe
```

## DXGI capture-only test (stable capture path)

```powershell
go build -o dxgi-capture-test.exe ./experimental/dxgi-capture-test
.\dxgi-capture-test.exe
```
