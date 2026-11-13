# Website clip: what zap removed, and bringing it back. The clean runs hidden first.
Output undo.mp4
Output undo.gif
Require zap
Set FontSize 16
Set Width 1200
Set Height 560
Set Padding 24
# Size first: vhs ignores Set commands that come after a Source.
Source settings.tape

Hide
Type "cd /tmp/zap-demo/home/dev/my-app && zap clean --all --yes >/dev/null 2>&1; clear" Enter
Sleep 3s
Show
Type "zap history" Sleep 300ms Enter
Sleep 3s
Type "zap undo" Sleep 300ms Enter
Sleep 4.5s
