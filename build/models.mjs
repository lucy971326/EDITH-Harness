// 开发时更新模型目录；日常构建与运行不请求 models.dev。
import { readFile, writeFile, rename } from 'node:fs/promises';
import { createHash } from 'node:crypto';

const source = 'https://models.dev/api.json';
const input = process.argv[2];
const raw = input ? await readFile(input, 'utf8') : await (async () => {
  const response = await fetch(source, { signal: AbortSignal.timeout(60000) });
  if (!response.ok) throw new Error(`models.dev: HTTP ${response.status}`);
  return response.text();
})();
const data = JSON.parse(raw);
const providers = {};
const models = {};
// 名单对齐本地 Pi；只收录 OpenAI Chat/Responses 与 Anthropic Messages。
// 不从数据源自动扩张供应商，也不把专有协议冒充 OpenAI 兼容接口。
const entries = [
  { id: 'ant-ling', name: 'Ant Ling', protocol: 'openai-chat', baseURL: 'https://api.ant-ling.com/v1' },
  { id: 'anthropic', name: 'Anthropic', protocol: 'anthropic', baseURL: 'https://api.anthropic.com' },
  { id: 'baseten', name: 'Baseten', protocol: 'openai-chat', baseURL: 'https://inference.baseten.co/v1' },
  { id: 'cerebras', name: 'Cerebras', protocol: 'openai-chat', baseURL: 'https://api.cerebras.ai/v1' },
  { id: 'cloudflare-ai-gateway', name: 'Cloudflare AI Gateway', protocol: 'openai-chat', baseURL: 'https://gateway.ai.cloudflare.com/v1/{account_id}/{gateway_id}' },
  { id: 'cloudflare-workers-ai', name: 'Cloudflare Workers AI', protocol: 'openai-chat', baseURL: 'https://api.cloudflare.com/client/v4/accounts/{account_id}/ai/v1' },
  { id: 'deepseek', name: 'DeepSeek', protocol: 'openai-chat', baseURL: 'https://api.deepseek.com', thinkingFormat: 'deepseek' },
  { id: 'fireworks', source: 'fireworks-ai', name: 'Fireworks', protocol: 'openai-chat', baseURL: 'https://api.fireworks.ai/inference/v1' },
  { id: 'github-copilot', name: 'GitHub Copilot', protocol: 'openai-chat', baseURL: 'https://api.individual.githubcopilot.com' },
  { id: 'groq', name: 'Groq', protocol: 'openai-chat', baseURL: 'https://api.groq.com/openai/v1' },
  { id: 'huggingface', name: 'Hugging Face', protocol: 'openai-chat', baseURL: 'https://router.huggingface.co/v1' },
  { id: 'kimi-coding', source: 'kimi-code-plan-global', name: 'Kimi For Coding', protocol: 'anthropic', baseURL: 'https://api.kimi.com/coding' },
  { id: 'meta', name: 'Meta', protocol: 'openai-responses', baseURL: 'https://api.meta.ai/v1' },
  { id: 'minimax', name: 'MiniMax', protocol: 'anthropic', baseURL: 'https://api.minimax.io/anthropic' },
  { id: 'minimax-cn', name: 'MiniMax CN', protocol: 'anthropic', baseURL: 'https://api.minimaxi.com/anthropic' },
  { id: 'moonshotai', name: 'Moonshot AI', protocol: 'openai-chat', baseURL: 'https://api.moonshot.ai/v1', thinkingFormat: 'deepseek' },
  { id: 'moonshotai-cn', name: 'Moonshot AI CN', protocol: 'openai-chat', baseURL: 'https://api.moonshot.cn/v1', thinkingFormat: 'deepseek' },
  { id: 'nvidia', name: 'NVIDIA', protocol: 'openai-chat', baseURL: 'https://integrate.api.nvidia.com/v1' },
  { id: 'openai', name: 'OpenAI', protocol: 'openai-responses', baseURL: 'https://api.openai.com/v1' },
  { id: 'opencode', name: 'OpenCode Zen', protocol: 'openai-chat', baseURL: 'https://opencode.ai/zen/v1' },
  { id: 'opencode-go', name: 'OpenCode Go', protocol: 'openai-chat', baseURL: 'https://opencode.ai/zen/go/v1' },
  { id: 'openrouter', name: 'OpenRouter', protocol: 'openai-chat', baseURL: 'https://openrouter.ai/api/v1', thinkingFormat: 'openrouter' },
  { id: 'qwen-token-plan', source: 'alibaba-token-plan', name: 'Qwen Token Plan', protocol: 'openai-chat', baseURL: 'https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1', thinkingFormat: 'qwen' },
  { id: 'qwen-token-plan-cn', source: 'alibaba-token-plan-cn', name: 'Qwen Token Plan CN', protocol: 'openai-chat', baseURL: 'https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1', thinkingFormat: 'qwen' },
  { id: 'qwen-token-plan-individual', source: 'alibaba-token-plan', name: 'Qwen Token Plan Individual', protocol: 'openai-chat', baseURL: 'https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1', thinkingFormat: 'qwen' },
  { id: 'together', source: 'togetherai', name: 'Together', protocol: 'openai-chat', baseURL: 'https://api.together.ai/v1' },
  { id: 'vercel-ai-gateway', source: 'vercel', name: 'Vercel AI Gateway', protocol: 'anthropic', baseURL: 'https://ai-gateway.vercel.sh' },
  { id: 'xai', name: 'xAI', protocol: 'openai-responses', baseURL: 'https://api.x.ai/v1' },
  { id: 'xiaomi', name: 'Xiaomi', protocol: 'openai-chat', baseURL: 'https://api.xiaomimimo.com/v1', thinkingFormat: 'deepseek' },
  { id: 'xiaomi-token-plan-cn', name: 'Xiaomi Token Plan CN', protocol: 'openai-chat', baseURL: 'https://token-plan-cn.xiaomimimo.com/v1', thinkingFormat: 'deepseek' },
  { id: 'xiaomi-token-plan-ams', name: 'Xiaomi Token Plan AMS', protocol: 'openai-chat', baseURL: 'https://token-plan-ams.xiaomimimo.com/v1', thinkingFormat: 'deepseek' },
  { id: 'xiaomi-token-plan-sgp', name: 'Xiaomi Token Plan SGP', protocol: 'openai-chat', baseURL: 'https://token-plan-sgp.xiaomimimo.com/v1', thinkingFormat: 'deepseek' },
  { id: 'zai', source: 'zai-coding-plan', name: 'Z.AI', protocol: 'openai-chat', baseURL: 'https://api.z.ai/api/coding/paas/v4', thinkingFormat: 'deepseek' },
  { id: 'zai-coding-cn', source: 'zhipuai-coding-plan', name: 'Z.AI Coding CN', protocol: 'openai-chat', baseURL: 'https://open.bigmodel.cn/api/coding/paas/v4', thinkingFormat: 'deepseek' },
];
const qwenIndividualModels = new Set(['deepseek-v4-flash-0731', 'deepseek-v4-pro', 'deepseek-v4-pro-0813', 'glm-5.2', 'qwen3.6-flash', 'qwen3.7-max', 'qwen3.7-plus', 'qwen3.8-flash', 'qwen3.8-max']);
const sorted = (object) => Object.fromEntries(Object.entries(object).sort(([a], [b]) => a.localeCompare(b, 'en')));

function levels(model, format, protocol) {
  if (!model.reasoning) return { off: {} };
  const options = model.reasoning_options ?? [];
  const effort = options.find((value) => value.type === 'effort');
  const toggle = options.some((value) => value.type === 'toggle');
  const budget = options.find((value) => value.type === 'budget_tokens');
  const result = {};
  const setting = (enabled, value) => {
    if (protocol === 'anthropic') {
      if (!enabled) return {};
      if (effort && !/claude-opus-4-5/.test(model.id)) return { thinking: { type: 'adaptive' }, effort: value };
      return { thinking: { type: 'enabled', budgetTokens: Math.max(1024, budget?.min ?? 1024) }, ...(effort ? { effort: value } : {}) };
    }
    if (format === 'qwen') return { enable_thinking: enabled, ...(value ? { reasoning_effort: value } : {}) };
    if (format === 'deepseek') return { thinking: { type: enabled ? 'enabled' : 'disabled' }, ...(value ? { reasoning_effort: value } : {}) };
    if (format === 'openrouter') return { reasoning: enabled ? (value ? { effort: value } : { enabled: true }) : { enabled: false } };
    return value ? { reasoning_effort: value } : {};
  };
  if (toggle || (protocol === 'anthropic' && budget)) result.off = setting(false);
  if (effort?.values?.length) {
    for (const value of effort.values) result[value === 'none' ? 'off' : value] = setting(value !== 'none', value);
  } else if (toggle || (protocol === 'anthropic' && budget)) {
    result.on = setting(true);
  } else result.auto = {};
  return result;
}

for (const { id, source: sourceID = id, ...preset } of entries) {
  providers[id] = preset;
  // models.dev 尚无 Ant Ling 独立目录，采用 Pi 明确维护的三个模型定义。
  if (id === 'ant-ling') {
    for (const modelID of ['Ling-2.6-flash', 'Ling-2.6-1T', 'Ring-2.6-1T']) {
      models[`${id}/${modelID}`] = { provider: id, id: modelID, contextWindow: 262144, maxOutput: 65536, vision: false,
        reasoning: modelID.startsWith('Ring') ? { auto: {} } : { off: {} } };
    }
    continue;
  }
  const sourceModels = data[sourceID]?.models;
  if (!sourceModels) throw new Error(`Missing provider source: ${sourceID}`);
  for (const [sourceModelID, model] of Object.entries(sorted(sourceModels))) {
    if (!model.tool_call || model.status === 'deprecated' || !model.modalities?.output?.includes('text') || /realtime|audio|tts|transcribe|deep-research/.test(sourceModelID)) continue;
    if (id.startsWith('qwen-token-plan') && sourceModelID === 'qwen3.8-max-preview') continue;
    if (id === 'qwen-token-plan-individual' && !qwenIndividualModels.has(sourceModelID)) continue;
    let modelID = sourceModelID;
    let protocol = preset.protocol;
    if (id === 'kimi-coding' && ['k2p5', 'k2p6', 'k2p7'].includes(modelID)) {
      if (sourceModels['kimi-for-coding']) continue;
      modelID = 'kimi-for-coding';
    }
    if (id === 'cloudflare-ai-gateway') {
      const [upstream, ...segments] = modelID.split('/');
      if (!['openai', 'anthropic', 'workers-ai'].includes(upstream)) continue;
      protocol = upstream === 'openai' ? 'openai-responses' : upstream === 'anthropic' ? 'anthropic' : 'openai-chat';
      if (upstream !== 'workers-ai') modelID = segments.join('/');
    }
    if (id === 'fireworks') protocol = /glm-|kimi-k3/.test(modelID) ? 'openai-chat' : 'anthropic';
    if (id === 'github-copilot') protocol = /^claude-(haiku|sonnet|opus|fable)-[45]([.\-]|$)/.test(modelID) ? 'anthropic'
      : /^(gpt-|grok-|oswe|mai-)/.test(modelID) ? 'openai-responses' : 'openai-chat';
    if (id === 'opencode' || id === 'opencode-go') {
      const npm = model.provider?.npm;
      if (npm === '@ai-sdk/google') continue;
      protocol = npm === '@ai-sdk/anthropic' ? 'anthropic' : npm === '@ai-sdk/openai' ? 'openai-responses' : 'openai-chat';
      if (id === 'opencode-go' && ['minimax-m2.7', 'qwen3.5-plus', 'qwen3.6-plus'].includes(modelID)) protocol = 'openai-chat';
    }
    if (!(model.limit?.context > 0 && model.limit?.output > 0)) throw new Error(`Missing limits: ${id}/${modelID}`);
    const reasoning = levels({ ...model, id: modelID }, preset.thinkingFormat, protocol);
    if (id === 'kimi-coding' && model.reasoning) {
      for (const options of Object.values(reasoning)) if (options.thinking) options.thinking = { type: 'adaptive' };
    }
    models[`${id}/${modelID}`] = { provider: id, id: modelID, ...(protocol !== preset.protocol ? { protocol } : {}),
      contextWindow: model.limit.context, maxOutput: model.limit.output,
      vision: model.modalities?.input?.includes('image') ?? false, reasoning };
  }
  if (!Object.values(models).some((model) => model.provider === id)) throw new Error(`Empty provider: ${id}`);
}
// 订阅通道有独立目录，不能把全部 OpenAI API 模型当成 ChatGPT 权限。
providers['openai-codex'] = { name: 'ChatGPT', protocol: 'openai-codex', baseURL: 'https://chatgpt.com/backend-api/codex' };
for (const id of ['gpt-6-astra', 'gpt-6-sol', 'gpt-6-luna', 'gpt-5.6-sol', 'gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.5', 'gpt-5.3-codex-spark']) {
  const model = models[`openai/${id}`];
  if (!model) throw new Error(`Missing Codex source: ${id}`);
  const reasoning = Object.fromEntries(Object.entries(model.reasoning).filter(([key]) => key !== 'off'));
  models[`openai-codex/${id}`] = { ...model, provider: 'openai-codex', contextWindow: id.endsWith('spark') ? 128000 : 272000, reasoning };
}
providers['xai-oauth'] = { ...providers.xai, name: 'xAI 账号' };
for (const model of Object.values(models)) {
  if (model.provider === 'xai') models[`xai-oauth/${model.id}`] = { ...model, provider: 'xai-oauth' };
}
const output = new URL('../internal/llm/catalog.json', import.meta.url);
const temporary = new URL('../internal/llm/catalog.json.tmp', import.meta.url);
await writeFile(temporary, JSON.stringify({ source, sourceSHA256: createHash('sha256').update(raw).digest('hex'), providers: sorted(providers), models: sorted(models) }, null, 2) + '\n');
await rename(temporary, output);
console.log(`Generated ${Object.keys(providers).length} providers, ${Object.keys(models).length} models.`);
