#!/bin/sh
# rptui launcher used by the Hyprland keybind and the desktop entry.
# Appends stderr to a state log so panic traces survive terminal closure.
exec /home/mds/Work/rptui-bubbletea/rptui --layout large 2>>"${XDG_STATE_HOME:-$HOME/.local/state}/rptui/stderr.log"
