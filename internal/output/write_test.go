package output

import (
	"strings"
	"testing"

	"github.com/bee/java-process-mapper/internal/flow"
	"github.com/bee/java-process-mapper/internal/model"
)

func TestBuildTraceGraphRoutesConditionalCalls(t *testing.T) {
	ev := func(line int, symbol string) model.Evidence {
		return model.Evidence{Path: "Controller.java", Line: line, Symbol: symbol, Kind: "test"}
	}
	project := &model.Project{
		Types: []model.Type{
			{
				ID:       "type:controller",
				Name:     "Controller",
				FQN:      "com.acme.Controller",
				FilePath: "Controller.java",
				Methods: []model.Method{
					{ID: "method:handle", TypeID: "type:controller", Name: "handle", Evidence: ev(1, "handle")},
				},
				Evidence: ev(1, "Controller"),
			},
			{
				ID:       "type:service",
				Name:     "Service",
				FQN:      "com.acme.Service",
				FilePath: "Service.java",
				Methods: []model.Method{
					{ID: "method:doWork", TypeID: "type:service", Name: "doWork", Evidence: model.Evidence{Path: "Service.java", Line: 1, Symbol: "doWork", Kind: "test"}},
				},
				Evidence: model.Evidence{Path: "Service.java", Line: 1, Symbol: "Service", Kind: "test"},
			},
		},
	}
	trace := flow.Trace{
		EntryPoint: model.EntryPoint{
			ID:       "entry:handle",
			Kind:     "http",
			Name:     "com.acme.Controller.handle",
			ClassID:  "type:controller",
			MethodID: "method:handle",
			Evidence: ev(1, "handle"),
		},
		Steps: []flow.Step{
			{
				Order:            1,
				Depth:            1,
				CallerClass:      "com.acme.Controller",
				CallerMethod:     "handle",
				Call:             "service.doWork",
				ResolvedClass:    "com.acme.Service",
				ResolvedMethod:   "doWork",
				ResolvedMethodID: "method:doWork",
				Resolution:       "field",
				Evidence:         ev(5, "service.doWork"),
				Source:           model.SourceFound,
				Confidence:       model.ConfidenceHigh,
			},
		},
		Conditions: []flow.ConditionUse{
			{
				Condition: model.Condition{
					ID:         "condition:with-call",
					MethodID:   "method:handle",
					Kind:       "if",
					Expression: "flag",
					StartLine:  4,
					BodyLine:   4,
					EndLine:    6,
					Evidence:   ev(4, "if flag"),
					Source:     model.SourceFound,
					Confidence: model.ConfidenceHigh,
				},
				CallerClass:  "com.acme.Controller",
				CallerMethod: "handle",
				Depth:        1,
			},
			{
				Condition: model.Condition{
					ID:         "condition:no-call",
					MethodID:   "method:handle",
					Kind:       "if",
					Expression: "noop",
					StartLine:  8,
					BodyLine:   8,
					EndLine:    9,
					Evidence:   ev(8, "if noop"),
					Source:     model.SourceFound,
					Confidence: model.ConfidenceHigh,
				},
				CallerClass:  "com.acme.Controller",
				CallerMethod: "handle",
				Depth:        1,
			},
		},
	}

	nodes, edges, auxiliary := buildTraceGraph(project, trace, newSourceLineReader(), newSourceContextStore())
	if len(auxiliary) != 0 {
		t.Fatalf("auxiliary steps = %d, want 0", len(auxiliary))
	}

	conditionID := ""
	noCallConditionID := ""
	conditionCount := 0
	for _, node := range nodes {
		if node.Kind != "condition" {
			continue
		}
		conditionCount++
		if strings.Contains(node.ID, "with-call") {
			conditionID = node.ID
			if node.Details["Conteudo"] != "" {
				t.Fatalf("condition node should not expose method content")
			}
		}
		if strings.Contains(node.ID, "no-call") {
			noCallConditionID = node.ID
			if node.Details["Conteudo"] != "" {
				t.Fatalf("condition node should not expose method content")
			}
		}
	}
	if conditionCount != 2 || conditionID == "" || noCallConditionID == "" {
		t.Fatalf("condition nodes = %d, rendered condition = %q, no-call condition = %q", conditionCount, conditionID, noCallConditionID)
	}

	foundConditionalCall := false
	for _, edge := range edges {
		if edge.Kind == "call" && edge.From == conditionID && edge.To == "step:1" {
			foundConditionalCall = true
		}
		if edge.Kind == "call" && edge.From == noCallConditionID {
			t.Fatalf("condition without internal calls should be rendered as context, not as a call parent")
		}
		if edge.Kind == "call" && strings.HasPrefix(edge.From, "entry:") && edge.To == "step:1" {
			t.Fatalf("conditional call should be routed through condition node, not directly from entrypoint")
		}
	}
	if !foundConditionalCall {
		t.Fatalf("expected call edge from condition %q to step:1", conditionID)
	}
}

func TestLayoutTraceGraphUsesCompactNodeBasedSpacing(t *testing.T) {
	nodes := []*traceGraphNode{
		{ID: "entry", Kind: "entry", Depth: 0, Row: 0},
		{ID: "step", Kind: "method", Depth: 1, Row: 1},
	}
	layoutTraceGraph(nodes)

	horizontalGap := nodes[1].X - (nodes[0].X + nodes[0].Width)
	if horizontalGap <= 0 || horizontalGap > 120 {
		t.Fatalf("horizontal gap = %d, want compact spacing based on neighboring node width", horizontalGap)
	}
	verticalGap := nodes[1].Y - (nodes[0].Y + nodes[0].Height)
	if verticalGap <= 0 || verticalGap > 120 {
		t.Fatalf("vertical gap = %d, want compact spacing based on neighboring node height", verticalGap)
	}

	anchor := &traceGraphNode{ID: "method", Kind: "method", Depth: 0, Row: 0}
	classNode := &traceGraphNode{ID: "class", Kind: "class", AnchorID: "method", AnchorPosition: "class"}
	layoutTraceGraph([]*traceGraphNode{anchor, classNode})
	classGap := anchor.Y - (classNode.Y + classNode.Height)
	if classGap <= 0 || classGap > 16 {
		t.Fatalf("class satellite gap = %d, want it close to its owning method", classGap)
	}
}
