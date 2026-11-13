# The README recording: every main command, in the order a first-time user meets them.
# Render with assets/demo/record.sh, never with vhs directly (see record.sh).
#
# The picker keys assume the fixture's order with --all: api's k2p9 (kept), then
# my-app's a1f3/my-app, auth and fix-login. Four Downs reach fix-login.
Output zap.gif
Require zap
Set FontSize 16
Set Width 1400
Set Height 860
Set Padding 28
# Size first: vhs ignores Set commands that come after a Source.
Source settings.tape

Hide
Type "cd /tmp/zap-demo/home/dev/my-app && clear" Enter
Show

Type "zap scan --all" Sleep 300ms Enter
Sleep 4.5s
Hide Type "clear" Enter Show

Type "zap map" Sleep 300ms Enter
Sleep 5s
Hide Type "clear" Enter Show

Type "zap check" Sleep 300ms Enter
Sleep 4s
Hide Type "clear" Enter Show

Type "zap explain .claude/worktrees/auth" Sleep 300ms Enter
Sleep 4.5s
Hide Type "clear" Enter Show

Type "zap clean --all --dry-run" Sleep 300ms Enter
Sleep 4.5s
Hide Type "clear" Enter Show

Type "zap --all" Sleep 300ms Enter
Sleep 2.5s
Down Sleep 400ms Down Sleep 400ms Down Sleep 400ms Down Sleep 700ms
Type "e" Sleep 3.5s
Escape Sleep 700ms
Up Sleep 300ms Up Sleep 300ms Up Sleep 300ms Up Sleep 700ms
Enter Sleep 3s
Type "y" Sleep 4.5s
Hide Type "clear" Enter Show

Type "zap undo" Sleep 300ms Enter
Sleep 4s
