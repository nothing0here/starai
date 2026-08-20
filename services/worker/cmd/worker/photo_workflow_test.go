package main

import (
	"strings"
	"testing"
)

// photoStudioMigrationStyles 与迁移 082 input_schema 中的风格枚举保持一致
var photoStudioMigrationStyles = []string{
	"影棚质感", "杂志大片", "黑白艺术", "韩系简约", "日系清新", "港风胶片", "法式复古", "美式复古",
	"国风古装", "旗袍风情", "新中式", "森系文艺", "咖啡馆日常", "都市夜景", "海边度假", "校园青春",
	"轻奢名媛", "甜美少女", "酷飒街头", "运动活力", "赛博霓虹", "暗调情绪", "户外自然", "婚纱浪漫",
	"雪景冬日", "商务精英", "纯白极简", "毕业季", "古典油画", "二次元动漫", "敦煌飞天", "民族风",
	"金秋落叶", "樱花春景", "Y2K千禧", "多巴胺糖果", "欧式宫廷", "沙漠戈壁",
}

func TestBuildPhotoStudioPromptConsumesAllParams(t *testing.T) {
	prompt := buildPhotoStudioPrompt("写真", "港风胶片", "白色", "穿白色连衣裙、回眸微笑")

	for _, want := range []string{"艺术写真", "港式复古胶片颗粒", "穿白色连衣裙、回眸微笑", "preserve the exact facial features"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt 缺少关键参数内容 %q：\n%s", want, prompt)
		}
	}
	// 非证件照不应出现底色指令
	if strings.Contains(prompt, "背景底色") {
		t.Fatalf("写真类型不应包含底色指令：%s", prompt)
	}
}

func TestBuildPhotoStudioPromptIDPhotoBackground(t *testing.T) {
	// 底色必须携带国内通行标准色值，避免模型自由发挥导致审核系统不认
	wantSpec := map[string]string{
		"白色": "RGB(255,255,255)",
		"蓝色": "RGB(67,142,219)",
		"红色": "RGB(255,0,0)",
	}
	for bg, spec := range wantSpec {
		prompt := buildPhotoStudioPrompt("证件照", "影棚质感", bg, "")
		if !strings.Contains(prompt, "标准证件照") {
			t.Fatalf("证件照缺少类型方向：%s", prompt)
		}
		if !strings.Contains(prompt, spec) {
			t.Fatalf("证件照底色 %q 缺少标准色值 %q：%s", bg, spec, prompt)
		}
	}
}

func TestBuildPhotoStudioPromptUnknownFallback(t *testing.T) {
	prompt := buildPhotoStudioPrompt("艺术人像", "水墨丹青", "", "")
	if !strings.Contains(prompt, "艺术人像") || !strings.Contains(prompt, "水墨丹青") {
		t.Fatalf("未知类型/风格应原样透传：%s", prompt)
	}
}

func TestPhotoStudioTypeDictionaryCoverage(t *testing.T) {
	for _, typ := range []string{"写真", "职业照", "证件照"} {
		if strings.TrimSpace(photoStudioTypes[typ]) == "" {
			t.Fatalf("写真类型字典缺少 %q", typ)
		}
	}
}

func TestPhotoStudioStyleDictionaryCoverage(t *testing.T) {
	if len(photoStudioMigrationStyles) != 38 {
		t.Fatalf("迁移风格枚举数量 = %d, want 38", len(photoStudioMigrationStyles))
	}
	for _, style := range photoStudioMigrationStyles {
		if strings.TrimSpace(photoStudioStyles[style]) == "" {
			t.Fatalf("风格字典缺少 %q", style)
		}
	}
}

func TestBuildPhotoStudioPromptOmitsEmptyUserPrompt(t *testing.T) {
	prompt := buildPhotoStudioPrompt("职业照", "商务精英", "", "   ")
	if strings.Contains(prompt, "额外要求") {
		t.Fatalf("空用户要求不应出现额外要求段：%s", prompt)
	}
}

func TestBuildPhotoStudioPromptIDPhotoWithoutStyle(t *testing.T) {
	prompt := buildPhotoStudioPrompt("证件照", "", "蓝色", "")
	if strings.Contains(prompt, "风格要求") {
		t.Fatalf("证件照未选风格时不应出现风格要求段：%s", prompt)
	}
	if !strings.Contains(prompt, "RGB(67,142,219)") {
		t.Fatalf("证件照缺少蓝底标准色值指令：%s", prompt)
	}
}
