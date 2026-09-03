package main

import (
	"fmt"
	"regexp"
	"strings"
)

// Some models put literal line breaks inside JSON script strings. Escape only
// those control bytes, preserving all content; never invent missing JSON fields.
func escapeComicJSONControls(text string) string {
	var out strings.Builder
	quoted, escaped := false, false
	for _, b := range []byte(text) {
		if escaped {
			out.WriteByte(b)
			escaped = false
			continue
		}
		if quoted && b == '\\' {
			out.WriteByte(b)
			escaped = true
			continue
		}
		if b == '"' {
			quoted = !quoted
		}
		if quoted && b < 0x20 {
			fmt.Fprintf(&out, `\u%04x`, b)
		} else {
			out.WriteByte(b)
		}
	}
	return out.String()
}

// Structural quality gate before spending on images/video. Semantic storytelling
// remains the LLM's job; an invalid plan must never become placeholder footage.
func validateComicDramaPlan(plan, inputs, runtimeCfg map[string]interface{}) error {
	shots := comicCollection(plan["storyboards"])
	grid := comicStoryboardGrid(runtimeCfg, inputs)
	if len(shots) != grid {
		return fmt.Errorf("需要%d个有效分镜，模型返回%d个，请重新规划该阶段", grid, len(shots))
	}
	seen := map[string]bool{}
	scenes := map[string]bool{}
	metadata := regexp.MustCompile(`当前用户要求|成品总秒数\s*[：:]|<agent_constraints>|承接上一镜，推进剧情至第`)
	for i, raw := range shots {
		shot, ok := raw.(map[string]interface{})
		if !ok {
			return fmt.Errorf("分镜%d结构错误", i+1)
		}
		id, scene := strings.TrimSpace(stringAny(shot["id"])), strings.TrimSpace(stringAny(shot["scene"]))
		if id == "" || seen[id] {
			return fmt.Errorf("分镜%d缺少唯一编号", i+1)
		}
		seen[id] = true
		if scene == "" || metadata.MatchString(scene) || scenes[scene] {
			return fmt.Errorf("分镜%s缺少独立剧情，或把参数说明当作画面", id)
		}
		scenes[scene] = true
		for _, key := range []string{"video_prompt", "keyframe_prompt", "dialogue", "narration"} {
			if metadata.MatchString(stringAny(shot[key])) {
				return fmt.Errorf("分镜%s的%s混入系统参数", id, key)
			}
		}
		// Speech length is not a structural error. The compositor measures the
		// generated audio and automatically reallocates shot time / adjusts tempo.
		// Rejecting it here strands a usable plan before that repair can run.
	}
	return nil
}
