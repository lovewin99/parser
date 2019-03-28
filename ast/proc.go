package ast

type CreateProcedureStmt struct {
	procNode

	//IfNotExists bool
	ProcName  string
	ProcParam string
	ProcBody  string
}

// Accept implements Node Accept interface.
func (n *CreateProcedureStmt) Accept(v Visitor) (Node, bool) {
	newNode, skipChildren := v.Enter(n)
	if skipChildren {
		return v.Leave(newNode)
	}
	n = newNode.(*CreateProcedureStmt)
	return v.Leave(n)
}
