# Shared by every demo tape: the look, and the throwaway HOME that record.sh builds.
# Never point HOME anywhere else, and never pass a ~ path on a command line: the
# shell would expand it before HOME applies.
Set Shell bash
Set FontFamily "JetBrains Mono"
Set Theme "Catppuccin Mocha"
Set TypingSpeed 40ms
Set CursorBlink false
Set Framerate 24
Env HOME "/tmp/zap-demo/home"
Env GIT_CONFIG_GLOBAL "/tmp/zap-demo/home/.gitconfig"
Env GIT_CONFIG_NOSYSTEM "1"
Env ZAP_AGENT_TMP "/tmp/zap-demo/home/tmp"
# vhs replaces PATH: keep the folders vhs itself needs (ttyd, ffmpeg) after the demo zap.
Env PATH "/tmp/zap-demo/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin:/usr/local/bin"
Env PS1 "\[\e[38;2;137;180;250m\]~/dev/my-app\[\e[0m\] $ "
Env BASH_SILENCE_DEPRECATION_WARNING "1"
