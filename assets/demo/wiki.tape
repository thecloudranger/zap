# Screenshots for the GitHub wiki: one still per command, in the order the wiki's
# beginner pages meet them. Record with assets/demo/wiki.sh, never with vhs directly.
# A screenshot needs a Sleep after it, or vhs captures the frame after the next command.
Output wiki/wiki.gif
Require zap
# Recorded at twice the size, and shown at half width, so text is sharp on high-DPI screens.
Set FontSize 32
Set Width 3600
Set Height 1720
Set Padding 56
# Size first: vhs ignores Set commands that come after a Source.
Source settings.tape

Hide
Type "git init -q /tmp/zap-demo/home/dev/new-app && cd /tmp/zap-demo/home/dev/my-app && clear" Enter
Show
Type "zap scan" Enter Sleep 4s
Screenshot wiki/shots/scan.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap scan --all" Enter Sleep 4.5s
Screenshot wiki/shots/scan-all.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap explain .claude/worktrees/auth" Enter Sleep 4s
Screenshot wiki/shots/explain.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap explain ../../.cursor/worktrees/api/k2p9" Enter Sleep 4s
Screenshot wiki/shots/explain-kept.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap clean --all --dry-run" Enter Sleep 4.5s
Screenshot wiki/shots/clean-dry-run.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap map" Enter Sleep 5s
Screenshot wiki/shots/map.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap check" Enter Sleep 4s
Screenshot wiki/shots/check.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap roots" Enter Sleep 2s
Screenshot wiki/shots/roots.png
Sleep 1s
Hide
Type `cd ../new-app && PS1='\[\e[38;2;137;180;250m\]~/dev/new-app\[\e[0m\] $ ' && clear` Enter
Show
Type "zap init" Enter Sleep 2s
Screenshot wiki/shots/init.png
Sleep 1s
# The first clean on a new machine: the disclaimer, then the confirmation.
Hide
Type `cd ../my-app && PS1='\[\e[38;2;137;180;250m\]~/dev/my-app\[\e[0m\] $ ' && rm /tmp/zap-demo/home/.config/zap/config.toml && clear` Enter
Show
Type "zap clean" Enter Sleep 4s
Screenshot wiki/shots/disclaimer.png
Sleep 1s
Type "zap" Sleep 300ms Enter Sleep 3s
Type "n" Sleep 300ms Enter Sleep 1s
Hide Type "clear" Enter Show
Type "zap clean" Enter Sleep 4s
Screenshot wiki/shots/clean-confirm.png
Sleep 1s
Type "y" Sleep 300ms Enter Sleep 5s
Screenshot wiki/shots/clean-done.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap history" Enter Sleep 2.5s
Screenshot wiki/shots/history.png
Sleep 1s
Hide Type "clear" Enter Show
Type "zap undo" Enter Sleep 4s
Screenshot wiki/shots/undo.png
Sleep 1s
