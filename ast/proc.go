package ast

type CreateProcedureStmt struct {
	procNode

	//IfNotExists bool
	ProcName  string
	ProcParam string
	ProcBody  []StmtNode
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

type CallProcedureStmt struct {
	procNode
	ProcName  string
	ProcParam string
}

// Accept implements Node Accept interface.
func (n *CallProcedureStmt) Accept(v Visitor) (Node, bool) {
	newNode, skipChildren := v.Enter(n)
	if skipChildren {
		return v.Leave(newNode)
	}
	n = newNode.(*CallProcedureStmt)
	return v.Leave(n)
}
