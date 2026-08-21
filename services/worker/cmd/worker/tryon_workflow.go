package main

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func processVirtualTryOnWorkflow(ctx context.Context, pool *pgxpool.Pool, baseURL, token string, p WorkflowTaskPayload, publicID string, estimated float64, inputs, runtimeCfg map[string]interface{}) error {
	outputs := loadWorkflowOutputs(ctx, pool, p.ProjectID)
	if _, done := outputs["media_tasks"]; done && stringAny(outputs["current_step"]) == "result" {
		return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
	}

	personURL := strings.TrimSpace(stringAny(inputs["person_image_url"]))
	garmentURL := strings.TrimSpace(stringAny(inputs["garment_image_url"]))
	if personURL == "" || garmentURL == "" {
		return failWorkflow(ctx, pool, p, publicID, estimated, "人物照片和服装图片不能为空")
	}

	modelCode := firstNonEmpty(stringAny(inputs["model_code"]), stringAny(runtimeCfg["generation_model_code"]))
	prompt := buildVirtualTryOnPrompt(stringAny(inputs["garment_category"]), stringAny(inputs["garment_photo_type"]), firstUserPrompt(inputs))
	genRuntime := copyMap(runtimeCfg)
	genRuntime["generation_model_code"] = modelCode
	genRuntime["generation_type"] = "image"
	genInputs := copyMap(inputs)
	genInputs["reference_images"] = []string{personURL, garmentURL}
	genInputs["count"] = positiveInt(intAny(inputs["count"]), 1)
	genInputs["n"] = genInputs["count"]
	genInputs["aspect_ratio"] = firstNonEmpty(stringAny(inputs["aspect_ratio"]), "3:4")
	genInputs["image_size"] = firstNonEmpty(stringAny(inputs["image_size"]), "1K")

	nodeRunID := insertWorkflowNodeRun(ctx, pool, p.ProjectID, "try_on", "AI试穿生成", "image", map[string]interface{}{
		"person_image_url":  personURL,
		"garment_image_url": garmentURL,
		"garment_category":  stringAny(inputs["garment_category"]),
		"model_code":        modelCode,
		"count":             genInputs["count"],
	}, 1)
	started := time.Now()
	results, errMsg := runAgentMediaTasks(ctx, pool, baseURL, token, p.ProjectID, p.UserID, publicID, genRuntime, genInputs, prompt)
	outputs["media_tasks"] = results
	outputs["current_step"] = "result"
	outputs["try_on"] = map[string]interface{}{
		"person_image_url":  personURL,
		"garment_image_url": garmentURL,
		"garment_category":  stringAny(inputs["garment_category"]),
		"model_code":        modelCode,
	}
	if errMsg != "" {
		saveWorkflowOutputs(ctx, pool, p.ProjectID, outputs)
		return failWorkflow(ctx, pool, p, publicID, estimated, "AI试穿生成失败："+errMsg)
	}
	updateNodeRunSuccess(ctx, pool, nodeRunID, map[string]interface{}{"count": len(results)}, 0, int(time.Since(started).Milliseconds()))
	return completeSimpleAgentWorkflow(ctx, pool, p, publicID, estimated, outputs)
}

func buildVirtualTryOnPrompt(category, photoType, userPrompt string) string {
	categoryRule := map[string]string{
		"tops":       "Replace only the upper-body garment. Preserve all lower-body clothing, shoes and accessories.",
		"bottoms":    "Replace only the lower-body garment. Preserve the top, shoes and accessories.",
		"one-pieces": "Replace the current main outfit with the one-piece garment. Preserve shoes and accessories unless they are occluded naturally.",
	}[category]
	if categoryRule == "" {
		categoryRule = "Detect the garment category from reference image 2 and replace only the corresponding clothing region."
	}
	photoRule := "Treat reference image 2 as the authoritative garment product image."
	if photoType == "flat-lay" {
		photoRule = "Reference image 2 is a flat-lay or ghost-mannequin product image. Ignore tags and the product-photo background while preserving the actual garment."
	} else if photoType == "model" {
		photoRule = "Reference image 2 shows the garment on another model. Transfer only the garment, never the second model's identity, body or pose."
	}

	parts := []string{
		"Reference image 1 is the authoritative person photo. Reference image 2 is the authoritative garment photo.",
		"Create a realistic virtual try-on image showing the person from reference image 1 wearing the exact garment from reference image 2.",
		"Strictly preserve the person's identity, facial features, hairstyle, skin tone, body shape, pose, hands, camera angle, background and lighting.",
		photoRule,
		categoryRule,
		"Preserve the garment's category, silhouette, cut, length, colors, patterns, fabric texture, buttons, pockets, seams, logos and printed text as accurately as possible.",
		"Do not change unrelated clothing or the background. Avoid distorted limbs, extra fingers, duplicated garments, warped logos, unreadable text and artificial skin.",
	}
	if strings.TrimSpace(userPrompt) != "" {
		parts = append(parts, "Additional wearing instruction: "+strings.TrimSpace(userPrompt)+". This instruction must not override person identity or garment fidelity.")
	}
	return strings.Join(parts, " ")
}

func positiveInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
