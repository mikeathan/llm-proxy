package profile

// SaveMode says who decides what is saved to memory during a run. It is the one place that answers it: the tools
// the model is offered, the save guidance, the pre-compaction flush and the capture of "remember that..." all read it.
type SaveMode string

const (
	// SaveModel lets the model decide: the write tools are offered and, in the operator's own chats, it is told when
	// to save. A profile that leaves the field out gets this, and so does the zero value.
	SaveModel SaveMode = "model"
	// SaveExplicit keeps the model from saving. Code saves what the user explicitly asks to remember; the write
	// tools are not offered, so no model can log its own task as a "fact".
	SaveExplicit SaveMode = "explicit"
	// SaveOff saves nothing, for runs nobody reviews or that must stay quiet.
	SaveOff SaveMode = "off"
)

const saveModeChoices = "model, explicit or off"

// Effective reads the unset value as the default, so every consumer sees one of the three modes.
func (m SaveMode) Effective() SaveMode {
	if m == "" {
		return SaveModel
	}
	return m
}

// Valid reports whether the mode is one a profile may declare (leaving it out is allowed).
func (m SaveMode) Valid() bool {
	return m == "" || m == SaveModel || m == SaveExplicit || m == SaveOff
}

// OffersWriteTools reports whether the model is offered memory_update and memory_forget.
func (m SaveMode) OffersWriteTools() bool { return m.Effective() == SaveModel }
