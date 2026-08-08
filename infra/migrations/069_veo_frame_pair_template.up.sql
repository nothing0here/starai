-- Split Veo first/last-frame generation from reference-image generation.
-- The standard -fl slug is less likely to be temporarily unavailable than fast-fl.
UPDATE models
SET new_api_model = 'veo_3_1-fl',
    new_api_endpoint = '/v1/videos',
    request_mode = 'video',
    category = 'video',
    description = 'VEO 3.1 首尾帧视频；支持仅首帧或首帧 + 尾帧，不支持参考图',
    input_schema = '{
      "type":"object",
      "properties":{
        "size":{
          "type":"string","title":"视频尺寸",
          "enum":["1280x720","720x1280","1920x1080","1080x1920"],
          "enumLabels":{"1280x720":"横屏 720P","720x1280":"竖屏 720P","1920x1080":"横屏 1080P","1080x1920":"竖屏 1080P"},
          "default":"1280x720","x-order":1,"x-widget":"option_menu","x-icon":"ratio","x-highlight":true
        }
      }
    }'::jsonb,
    default_params = '{"size":"1280x720"}'::jsonb,
    runtime_rule = '{
      "video":{
        "upload_profile":"veo_frame_pair",
        "min_reference_images":0,
        "max_reference_images":0,
        "max_total_images":2,
        "count_toward_total":true,
        "prompt_required":true,
        "prompt_hint":"上传 1 张首帧，或同时上传首帧和尾帧；本模板不支持参考图",
        "show_channel":false,
        "show_web_search":false,
        "count_options":[1],
        "count_allow_custom":false,
        "count_max":1,
        "frames":{
          "first":{"key":"first_frame","label":"首帧","max":1},
          "last":{"key":"last_frame","label":"尾帧（可选）","max":1}
        },
        "reference_images":{"key":"reference_images","max":0}
      },
      "upstream":{
        "adapter":"veo_frame_pair_v1",
        "include":["size","first_frame","last_frame"],
        "map":{},
        "poll_path":"/v1/videos/{id}",
        "poll_interval_sec":10,
        "poll_timeout_sec":7200,
        "request_timeout_sec":120
      },
      "capabilities":{"web_search":false,"deep_think":false}
    }'::jsonb,
    updated_at = now()
WHERE code = 'video_veo3_1';
