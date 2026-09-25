package sessioninbox

// GateReasonText is the host's own sentence for a gate: a sender relays it
// without parsing it, so it is never the machine-readable answer. An unknown
// or empty gate says nothing.
func GateReasonText(gate string) string {
	switch gate {
	case GateAwaitingAnswer:
		return "这个会话正在等人回答或审批，先处理掉那个提示才会轮到这一条。"
	case GateTurnRunning:
		return "它当前那一轮还没跑完，跑完就会轮到这一条。"
	case GateTurnFinishing:
		return "它这一轮正在收尾，收完就会轮到这一条。"
	case GateRotating:
		return "它正在切换会话，切换完就会轮到这一条。"
	case GateClosed:
		return "这个会话已经关闭，队列不会再被派发。"
	case GateNoSessionPath:
		return "这个会话没有可落地的会话文件，队列无处存放。"
	case GatePaused:
		return "它的队列被暂停了，恢复后才会派发。"
	case GateReadonly:
		return "它的队列是只读的，这一条不会被派发。"
	case GateHostDispatch:
		return "下一脚派发归宿主（桌面发布钩子）所有，会话这一侧看不到它何时放行。"
	default:
		return ""
	}
}

// GateWaitsForUser reports whether a gate is the class the human clears
// themselves, rather than one that opens on its own.
func GateWaitsForUser(gate string) bool { return gate == GateAwaitingAnswer }
