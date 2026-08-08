UPDATE models
SET new_api_model = 'veo-3.1',
    new_api_endpoint = '/v1/video/generations',
    description = '首尾帧 + 参考图，支持生成模式/比例/提示词优化/超分',
    input_schema = '{
      "type":"object",
      "properties":{
        "count":{"type":"integer","title":"生成数量","enum":[1,2,3],"default":1,"x-order":1,"x-widget":"option_menu","x-icon":"layers","x-highlight":true},
        "generation_mode":{"type":"string","title":"生成模式","enum":["fast","standard","high"],"enumLabels":{"fast":"快速","standard":"标准","high":"高资质"},"default":"standard","x-order":2,"x-widget":"option_menu","x-icon":"sparkles"},
        "aspect_ratio":{"type":"string","title":"视频比例","enum":["9:16","16:9","1:1"],"default":"9:16","x-order":3,"x-widget":"option_menu","x-icon":"ratio"},
        "prompt_enhance":{"type":"boolean","title":"提示词优化","default":true,"x-order":4,"x-widget":"boolean_toggle","x-icon":"wand"},
        "upscale":{"type":"boolean","title":"视频超分","default":false,"x-order":5,"x-widget":"boolean_toggle","x-icon":"4k"}
      }
    }'::jsonb,
    default_params = '{"count":1,"generation_mode":"standard","aspect_ratio":"9:16","prompt_enhance":true,"upscale":false}'::jsonb,
    runtime_rule = '{
      "video":{
        "upload_profile":"frame_pair",
        "frames":{"first":{"key":"first_frame","label":"首帧","max":1},"last":{"key":"last_frame","label":"尾帧","max":1}},
        "reference_images":{"key":"reference_images","max":4},
        "max_total_images":6,
        "count_toward_total":true,
        "prompt_required":true,
        "show_channel":true
      },
      "upstream":{
        "include":["count","generation_mode","aspect_ratio","prompt_enhance","upscale","first_frame","last_frame","reference_images"],
        "map":{"count":"n","prompt_enhance":"prompt_optimizer","upscale":"super_resolution"}
      }
    }'::jsonb,
    updated_at = now()
WHERE code = 'video_veo3_1';
