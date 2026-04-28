package main

import (
	"fmt"
	"strings"
)

func generateTypeHelpers(w *strings.Builder, t TypeInfo, moduleName string) {
	mtName := moduleName + "." + t.GoName

	// push helper: pushPlayer(L, v)
	w.WriteString(fmt.Sprintf("func push%s(L *lua.State, v *%s) {\n", t.GoName, t.GoName))
	w.WriteString("\tL.PushUserdata(v)\n")
	w.WriteString(fmt.Sprintf("\tL.GetField(lua.RegistryIndex, %q)\n", mtName))
	w.WriteString("\tL.SetMetatable(-2)\n")
	w.WriteString("}\n\n")

	// check helper: checkPlayer(L, idx)
	w.WriteString(fmt.Sprintf("func check%s(L *lua.State, idx int) *%s {\n", t.GoName, t.GoName))
	w.WriteString(fmt.Sprintf("\tL.CheckUdata(idx, %q)\n", mtName))
	w.WriteString(fmt.Sprintf("\treturn L.UserdataValue(idx).(*%s)\n", t.GoName))
	w.WriteString("}\n\n")
}

func generateFieldAccessors(w *strings.Builder, t TypeInfo, moduleName string) {
	mtName := moduleName + "." + t.GoName

	// __index metamethod
	w.WriteString(fmt.Sprintf("func lua_%s__index(L *lua.State) int {\n", t.GoName))
	w.WriteString(fmt.Sprintf("\tself := check%s(L, 1)\n", t.GoName))
	w.WriteString("\tkey := L.CheckString(2)\n")
	w.WriteString("\tswitch key {\n")
	for _, f := range t.Fields {
		w.WriteString(fmt.Sprintf("\tcase %q:\n", f.LuaName))
		w.WriteString(fmt.Sprintf("\t\t%s\n", genPushExpr(ReturnInfo{GoType: f.GoType}, "self."+f.GoName)))
	}
	// Methods fallback — check metatable
	w.WriteString("\tdefault:\n")
	w.WriteString(fmt.Sprintf("\t\tL.GetField(lua.RegistryIndex, %q)\n", mtName))
	w.WriteString("\t\tL.GetField(-1, key)\n")
	w.WriteString("\t\treturn 1\n")
	w.WriteString("\t}\n")
	w.WriteString("\treturn 1\n")
	w.WriteString("}\n\n")

	// __newindex metamethod
	w.WriteString(fmt.Sprintf("func lua_%s__newindex(L *lua.State) int {\n", t.GoName))
	w.WriteString(fmt.Sprintf("\tself := check%s(L, 1)\n", t.GoName))
	w.WriteString("\tkey := L.CheckString(2)\n")
	w.WriteString("\tswitch key {\n")
	for _, f := range t.Fields {
		if f.ReadOnly {
			w.WriteString(fmt.Sprintf("\tcase %q:\n", f.LuaName))
			w.WriteString("\t\treturn L.Errorf(\"field %q is read-only\", key)\n")
			continue
		}
		w.WriteString(fmt.Sprintf("\tcase %q:\n", f.LuaName))
		w.WriteString(fmt.Sprintf("\t\tself.%s = %s\n", f.GoName, genCheckExpr(ParamInfo{GoType: f.GoType}, 3)))
	}
	w.WriteString("\tdefault:\n")
	w.WriteString("\t\treturn L.Errorf(\"unknown field %q\", key)\n")
	w.WriteString("\t}\n")
	w.WriteString("\treturn 0\n")
	w.WriteString("}\n\n")
}
