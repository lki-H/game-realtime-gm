package experiment

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unicode/utf16"
)

func secureAuditPath(path string, directory bool) error {
	// Verify ownership before changing an existing path's ACL. A fresh protected
	// ACL then excludes inherited and unrelated explicit entries.
	// Values travel in environment variables, never as PowerShell source.
	script := `$ErrorActionPreference='Stop'; $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User; $attributes=[IO.File]::GetAttributes($env:R5_AUDIT_PATH); if(($attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0){throw 'Audit reparse path rejected'}; if($env:R5_AUDIT_IS_DIRECTORY -eq 'true'){ $existing=[IO.Directory]::GetAccessControl($env:R5_AUDIT_PATH,[Security.AccessControl.AccessControlSections]::Owner); $acl=[Security.AccessControl.DirectorySecurity]::new(); $inherit=[Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit' }else{ $existing=[IO.File]::GetAccessControl($env:R5_AUDIT_PATH,[Security.AccessControl.AccessControlSections]::Owner); $acl=[Security.AccessControl.FileSecurity]::new(); $inherit=[Security.AccessControl.InheritanceFlags]::None }; if(-not $existing.GetOwner([Security.Principal.SecurityIdentifier]).Equals($sid)){throw 'Audit current-user ownership required'}; $acl.SetAccessRuleProtection($true,$false); foreach($principal in @($sid,[Security.Principal.SecurityIdentifier]::new('S-1-5-18'))){ $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($principal,[Security.AccessControl.FileSystemRights]::FullControl,$inherit,[Security.AccessControl.PropagationFlags]::None,[Security.AccessControl.AccessControlType]::Allow)) }; if($env:R5_AUDIT_IS_DIRECTORY -eq 'true'){ [IO.Directory]::SetAccessControl($env:R5_AUDIT_PATH,$acl) }else{ [IO.File]::SetAccessControl($env:R5_AUDIT_PATH,$acl) }`
	characters := utf16.Encode([]rune(script))
	data := make([]byte, len(characters)*2)
	for index, character := range characters {
		binary.LittleEndian.PutUint16(data[index*2:], character)
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(data))
	command.Env = append(os.Environ(), "R5_AUDIT_PATH="+path, "R5_AUDIT_IS_DIRECTORY="+strconv.FormatBool(directory))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Run()
}
