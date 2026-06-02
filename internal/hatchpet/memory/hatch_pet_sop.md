# hatch_pet_sop

目标：把角色/品牌/参考图/文字概念制作成 Codex 兼容 animated pet：`${CODEX_HOME:-$HOME/.codex}/pets/<pet-name>/pet.json` + `spritesheet.webp`。本 SOP 是 Codex `hatch-pet` skill 的 GA 适配版。

## 0. 适用与硬边界
- 适用：用户要制作/修复/验证/打包 Codex pet、9 行 spritesheet、pet atlas。
- 必须使用 Codex hatch-pet 的确定性脚本做 atlas/QA/打包；脚本位置先用 `es prepare_pet_run.py` 查，已见常见路径：`C:\Users\Fwind43\.codex\skills\hatch-pet\scripts\`。
- 视觉生成只能由图像生成模型/worker 产出；禁止用本地脚本“画/拼/伪造”base 或 row strip。
- 只有 `base` 可纯 prompt；每个 row job 必须带 manifest 指定 input images（通常含 canonical base + layout guide）。
- GA 环境没有 Codex `$imagegen` 内置命令时，按 `gpt_image_2_sop.md` 走 `gpt-image-2` 图片生成；生成返回图片后由父 agent 复制到 manifest 指定 decoded 路径。
- 品牌仅做 mascot-safe 灵感：不要复制 logo、可读文字、UI 截图、口号，除非用户明确给定并授权。

## 1. 9 行状态契约
Codex app 当前使用全部 9 个状态：
`idle`, `running-right`, `running-left`, `waving`, `jumping`, `failed`, `waiting`, `running`, `review`。

- `running-left` 是唯一可确定性派生的视觉行：只能在 `running-right` 已生成、视觉检查并确认镜像不会破坏身份/语义后，用脚本派生；否则正常生成。
- 不要整条 strip 水平镜像来替代脚本；脚本会逐帧镜像并保持时序。

## 2. 风格与透明规则
- 默认 style=`auto`，也可接受 `pixel`, `plush`, `clay`, `sticker`, `flat-vector`, `3d-toy`, `painterly`, `brand-inspired`。
- pet-safe 标准：192x208 单元格内可读的完整小体型轮廓；脸、比例、材质、配色、道具全行一致；干净可移除 chroma-key 背景；无文字/标签/UI/可读 logo。
- 效果必须附着/接触 pet 轮廓、在同一 frame slot 内、不形成独立组件、硬边不透明、非 chroma-key 色、小而可读。
- 默认避免：波纹/速度线/拖影/模糊、漂浮星星/标点/图标/烟尘、阴影/地面/光晕、文本/UI/棋盘透明、背景、接近 chroma-key 的宠物颜色、 stray pixels、裁切、跨格姿势。
- 状态要点：
  - `idle` 仅微小呼吸/眨眼/轻晃；不能六帧几乎相同，也不能做其他状态动作。
  - `waving` 只用肢体姿势表达挥手；不要波纹/线条/符号。
  - `jumping` 只用身体高度表达；不要阴影/落地尘/地面 cues。
  - `failed` 可有附着的不透明泪/烟/星；不要红 X 或漂浮符号。
  - `waiting` 表达等待用户/审批，与 idle/review 区分。
  - `running` 表达工作/思考/扫描/打字等任务进行中；不要真的跑步/方向移动/速度线。
  - `review` 用凝视/倾身/眨眼/头部/手势表达专注；不要新增放大镜/纸/code/UI/标点。
  - `running-right/left` 必须方向正确、步态交替明显；不要速度线/尘土/地面阴影/拖影。

## 3. 推荐 GA 工作流
1. 建可见 checklist/plan，逐步更新：收集输入→prepare run→生成 base→生成/派生 9 rows→确定性处理→最终视觉 QA→打包/清理。
2. 若用户只给品牌/公司/产品名：先做品牌发现（web 搜索或让 subagent 输出 compact brief）；无 web 或只有裸品牌名且无视觉线索时，先问用户要品牌 cues。
3. 运行 prepare：
```bash
SKILL_DIR="/absolute/path/to/hatch-pet"
python "$SKILL_DIR/scripts/prepare_pet_run.py" \
  --output-root "/absolute/output/root" \
  --pet-notes "<用户概念/角色说明>" \
  --style auto \
  --reference "/absolute/ref.png" \
  --brand-name "<可选>" \
  --brand-brief "<可选短brief>" \
  --brand-source "<可选URL>"
```
参数按实际输入增减；文本-only 可只传 `--pet-notes`。prepare 会生成 run 目录、`pet_request.json`、`imagegen-jobs.json`、prompts、layout guides。
4. 查看 ready jobs：
```bash
jq '.jobs[] | {id, kind, status, depends_on, prompt_file, retry_prompt_file, input_images, output_path, derivation_policy}' "$RUN_DIR/imagegen-jobs.json"
```
ready = 未 complete 且 `depends_on` 全 complete。
5. 生成顺序：先 `base`；再 `idle` + `running-right` 做身份/步态检查；安全时派生 `running-left`，不安全则生成；再生成剩余 rows。默认最多 2 个生成 worker 并行。
6. 每个 job 由 worker/图像模型生成后，父 agent 复制选中图到 decoded，并更新 manifest；不要让 worker 改 manifest：
```bash
RUN_DIR=/absolute/path/to/run
JOB_ID=<job-id>
SOURCE=/absolute/path/to/generated-output.png
OUTPUT_REL=$(jq -r --arg id "$JOB_ID" '.jobs[] | select(.id == $id) | .output_path' "$RUN_DIR/imagegen-jobs.json")
mkdir -p "$(dirname "$RUN_DIR/$OUTPUT_REL")"
cp "$SOURCE" "$RUN_DIR/$OUTPUT_REL"
if [ "$JOB_ID" = "base" ]; then mkdir -p "$RUN_DIR/references"; cp "$RUN_DIR/$OUTPUT_REL" "$RUN_DIR/references/canonical-base.png"; fi
UPDATED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
TMP_MANIFEST=$(mktemp)
jq --arg id "$JOB_ID" --arg source "$SOURCE" --arg at "$UPDATED_AT" '(.jobs[] | select(.id == $id)) += {status: "complete", source_path: $source, completed_at: $at}' "$RUN_DIR/imagegen-jobs.json" > "$TMP_MANIFEST"
mv "$TMP_MANIFEST" "$RUN_DIR/imagegen-jobs.json"
```
7. 如果 row 生成 Bad Request：同一 row 用 `retry_prompt_file` + 同 input images 重试一次；仍失败就停止并报告 row id 与 prompt 路径，禁止换本地伪造路径。
8. 安全派生 running-left：
```bash
python "$SKILL_DIR/scripts/derive_running_left_from_running_right.py" \
  --run-dir "$RUN_DIR" \
  --confirm-appropriate-mirror \
  --decision-note "<why mirroring preserves identity>"
```

## 4. 确定性处理与验证
所有 jobs complete 后执行：
```bash
mkdir -p "$RUN_DIR/final" "$RUN_DIR/qa"
python "$SKILL_DIR/scripts/extract_strip_frames.py" --decoded-dir "$RUN_DIR/decoded" --output-dir "$RUN_DIR/frames" --states all --method auto
python "$SKILL_DIR/scripts/inspect_frames.py" --frames-root "$RUN_DIR/frames" --json-out "$RUN_DIR/qa/review.json" --require-components
python "$SKILL_DIR/scripts/compose_atlas.py" --frames-root "$RUN_DIR/frames" --output "$RUN_DIR/final/spritesheet.png" --webp-output "$RUN_DIR/final/spritesheet.webp"
python "$SKILL_DIR/scripts/validate_atlas.py" "$RUN_DIR/final/spritesheet.webp" --json-out "$RUN_DIR/final/validation.json"
python "$SKILL_DIR/scripts/make_contact_sheet.py" "$RUN_DIR/final/spritesheet.webp" --output "$RUN_DIR/qa/contact-sheet.png"
python "$SKILL_DIR/scripts/render_animation_previews.py" --frames-root "$RUN_DIR/frames" --output-dir "$RUN_DIR/qa/previews"
```
若 preview GIF 显示的 size popping/baseline jump 是抽帧 fit-to-cell 导致，且原始 row strip 本身比例/位置稳定，可改用 `stable-slots` 重跑 extract→inspect→compose→validate→contact sheet→previews：
```bash
python "$SKILL_DIR/scripts/extract_strip_frames.py" --decoded-dir "$RUN_DIR/decoded" --output-dir "$RUN_DIR/frames" --states all --method stable-slots
python "$SKILL_DIR/scripts/inspect_frames.py" --frames-root "$RUN_DIR/frames" --json-out "$RUN_DIR/qa/review.json" --require-components --allow-stable-slots
```
`stable-slots` 只作 QA 驱动修正，不作默认掩盖坏 row。

## 5. 子 agent/worker 分工（GA 适配）
- 先读 `subagent.md`；可用 map 模式分发 base/row/final QA。大量图片/视觉判断默认用轻量 subagent，父 agent 做编排与文件写入。
- 父 agent：品牌发现、prepare、读 manifest、分派、复制 selected_source、更新 manifest、建 canonical base、派生镜像、跑脚本、打包、修复、清理。
- base worker：只处理 base prompt 和列出的参考图；只返回：
```text
selected_source=/absolute/path/to/selected-output.png
qa_note=<one sentence>
```
- row worker：只处理一个 row；读 prompt/retry prompt 和所有 input images；Bad Request 可重试一次；检查帧数、身份、chroma 背景、间距、裁切、detached effects/guide marks；只返回同上两行。
- final QA worker：只检查 `qa/contact-sheet.png`、`qa/previews/*.gif`，必要时参考 `qa/review.json` 与 `final/validation.json`；返回：
```text
visual_qa=pass|fail
qa_note=<one sentence summary>
repairs=<row-id: note; ... or empty>
```
不得让 worker 编辑文件、改 manifest、打包或清理。

## 6. 修复流程
- `qa/review.json` errors 是 blocker；warnings 需要视觉复核。
- 最终视觉 QA 失败时，读 `qa/review.json` 和 QA notes，只重生成最小失败范围；身份修复要带 canonical base、原参考、contact sheet、失败说明。
- 替换 row 时复制到原 decoded output_path，并保持/更新该 job complete source_path；然后重跑确定性处理和最终 QA。
- 身份/风格漂移即使 validation 无错误也必须阻塞。

## 7. 打包与 summary
通过 deterministic validation + visual QA 后打包：
```bash
PET_ID=$(jq -r '.pet_id' "$RUN_DIR/pet_request.json")
DISPLAY_NAME=$(jq -r '.display_name' "$RUN_DIR/pet_request.json")
DESCRIPTION=$(jq -r '.description' "$RUN_DIR/pet_request.json")
PET_DIR="${CODEX_HOME:-$HOME/.codex}/pets/$PET_ID"
mkdir -p "$PET_DIR"
cp "$RUN_DIR/final/spritesheet.webp" "$PET_DIR/spritesheet.webp"
jq -n --arg id "$PET_ID" --arg displayName "$DISPLAY_NAME" --arg description "$DESCRIPTION" '{id: $id, displayName: $displayName, description: $description, spritesheetPath: "spritesheet.webp"}' > "$PET_DIR/pet.json"
jq -n --arg run_dir "$RUN_DIR" --arg spritesheet "$RUN_DIR/final/spritesheet.webp" --arg validation "$RUN_DIR/final/validation.json" --arg contact_sheet "$RUN_DIR/qa/contact-sheet.png" --arg review "$RUN_DIR/qa/review.json" --arg package "$PET_DIR" '{ok: true, run_dir: $run_dir, spritesheet: $spritesheet, validation: $validation, contact_sheet: $contact_sheet, review: $review, package: $package}' > "$RUN_DIR/qa/run-summary.json"
```

## 8. 清理策略
视觉 QA 通过后可清理中间物；若用户要求 debug 或仍需 repair 则跳过。
保留：`pet_request.json`, `final/spritesheet.webp`, `final/validation.json`, `qa/contact-sheet.png`, `qa/previews/`, `qa/review.json`, `qa/run-summary.json`。
可删：prompts、layout guides、decoded row strips、frames、`final/spritesheet.png`、`imagegen-jobs.json`。

## 9. 验收标准
- `pet.json` 与 `spritesheet.webp` 同目录 staged。
- 全 9 行状态齐全，frame count/尺寸/透明像素 invariant 通过脚本验证。
- `qa/review.json` 无 errors；final visual QA pass。
- 所有行同一宠物身份、风格、配色、材质、道具；方向行朝向/步态正确；idle 有微变化但不越界。
- 预览 GIF 无非预期大小跳变、基线跳、反向 cadence 或状态语义错误。
