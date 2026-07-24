# PROTOTYPE — potato shell hand-off demo (wayfinder ticket #6).
#
#   source potato-proto.zsh
#   pp
#
# Enter in the TUI pre-fills your prompt with the selected command;
# you press Enter again to actually execute it. Nothing runs by itself.
# This is the `print -z` flavour of the hand-off decided in ticket #3.

_potato_proto_dir="${${(%):-%x}:A:h}"

pp() {
  local cmd
  cmd="$( (cd "$_potato_proto_dir" && bun run index.tsx) )" || return
  [[ -n "$cmd" ]] && print -z -- "$cmd"
}
