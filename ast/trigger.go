package ast

type CreateTriggerStmt struct {
	triggerNode

	Name            string
	TriggerTime     string
	TriggerEvent    string
	TargetSchema    string
	TriggerStmt     []StmtNode
	TriggerStmtStrs string
}

// Accept implements Node Accept interface.
func (n *CreateTriggerStmt) Accept(v Visitor) (Node, bool) {
	newNode, skipChildren := v.Enter(n)
	if skipChildren {
		return v.Leave(newNode)
	}
	n = newNode.(*CreateTriggerStmt)
	return v.Leave(n)
}
