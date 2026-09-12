import type { Project } from '@/lib/types';

export const occasions: Record<string, string> = {
  opening: '开场 / 新人入场前',
  story: '仪式 / 爱情回顾',
  warmup: '入席 / 暖场循环',
  heritage: '家族口述史',
  tribute: '长辈纪念',
  tradition: '家风传承',
  anniversary: '周年纪念',
  memories: '恋爱回忆',
  distance: '异地寄语',
  product: '商品展示',
  tutorial: '使用演示',
  brand: '品牌故事',
};

export const occasionHints: Record<string, string> = {
  heritage: '用真实口述与老物件连接时代记忆。',
  tribute: '记录长辈的人生片段与亲友寄语。',
  tradition: '用生活细节表达家风与代际传承。',
  anniversary: '以纪念日为线索，回看陪伴与成长。',
  memories: '将共同回忆编排成温暖的日常故事。',
  distance: '跨越距离，用影像传递思念与期待。',
  product: '突出商品外观、材质和已证实的卖点。',
  tutorial: '清晰呈现真实使用步骤与应用场景。',
  brand: '结合品牌资料讲述理念与产品价值。',
  opening:
    '抓住宾客注意 → 关系推进 → 正式亮相。片尾留画、降音乐，方便主持人接话。',
  story: '以真实经历串起关系变化，最后感谢亲友；不发出立即入场口令。',
  warmup: '轻松、可重复观看；播放器自动循环，片尾不提示新人立即入场。',
};

export const promptExamples = [
  {
    name: '三幕变装',
    text: '开场用日常装的轻松互动，中段转为红色中式礼服，最后婚纱与黑色西装正式亮相。只在章节交界变装，用同一束花遮挡切接，不要人体变形。结尾把注意力交还现场。',
  },
  {
    name: '有故事的开场',
    text: '用一张空白信笺贯穿相遇、陪伴与今天：递出信笺、共同折起、收进胸前口袋。不要只拍海边摆拍，不编造相恋日期；少量大字字幕，给主持人留3秒接话。',
  },
  {
    name: '中式到现代',
    text: '青石小院的红色中式礼服，接到现代花园的婚纱西装。用相同转身方向衔接换装。配乐由轻拨弦发展到弦乐与鼓组，中间留一个安静桥段；不要群像与碰杯。',
  },
];

export const styleChoices = [
  {
    id: 'joyful',
    name: '欢快庆典',
    hint: '明亮色彩、笑闹互动、轻快节拍',
    bpm: 120,
  },
  {
    id: 'romantic',
    name: '浪漫电影',
    hint: '亲密细节、柔和光线、弦乐起伏',
    bpm: 96,
  },
  {
    id: 'vintage',
    name: '复古胶片',
    hint: '暖调抓拍、轻盈摇摆、爵士色彩',
    bpm: 108,
  },
  {
    id: 'epic',
    name: '史诗仪式',
    hint: '空间层次、庄重仪式、管弦高潮',
    bpm: 96,
  },
  {
    id: 'travel',
    name: '旅行纪实',
    hint: '连贯移动、自然互动、自由感',
    bpm: 120,
  },
  {
    id: 'editorial',
    name: '时尚短片',
    hint: '利落构图、节奏切镜、视觉张力',
    bpm: 120,
  },
];

export type ProjectDraft = {
  creationMode: string;
  templateId: string;
  scene: string;
  title: string;
  brief: string;
  occasion: string;
  customPrompt: string;
  wardrobeMode: string;
  wardrobePrompt: string;
  endingText: string;
  duration: number;
  style: string;
  ratio: string;
};

export const creativeDefaults = {
  creationMode: 'agent',
  templateId: '',
  scene: 'wedding',
  occasion: 'opening',
  customPrompt: '',
  wardrobeMode: 'auto',
  wardrobePrompt: '',
  endingText: '',
};

export function defaultDraft(): ProjectDraft {
  return {
    ...creativeDefaults,
    title: '',
    brief: '',
    duration: 60,
    style: 'joyful',
    ratio: '16:9',
  };
}

export const sceneChoices = [
  {
    id: 'wedding',
    name: '婚礼影片',
    eyebrow: 'WEDDING',
    description: '从相遇到相守，为现场留一个动人的开场。',
    occasions: ['opening', 'story', 'warmup'],
    briefLabel: '你们的故事与事实素材',
    briefHint: '相遇、共同爱好与想感谢的人；只填写真实经历。',
    titleHint: '例如：把余生写成我们',
    prompt:
      '用一束花串起相遇、陪伴与今天，在章节交界衔接造型，片尾留三秒给主持人。',
    duration: 60,
    style: 'joyful',
    ratio: '16:9',
  },
  {
    id: 'family',
    name: '家族传承',
    eyebrow: 'FAMILY LEGACY',
    description: '保存长辈的故事，让记忆在代际之间延续。',
    occasions: ['heritage', 'tribute', 'tradition'],
    briefLabel: '家族故事与资料',
    briefHint:
      '人物关系、老照片背景、真实年代、口述片段与家族物件；不确定的事实请注明。',
    titleHint: '例如：家的来处',
    prompt:
      '以老相册、手写家书和一张饭桌串起三代人的记忆。只使用提供的家史和年代，温暖克制，不虚构人物经历，不安排婚礼。',
    duration: 120,
    style: 'vintage',
    ratio: '16:9',
  },
  {
    id: 'anniversary',
    name: '爱情纪念',
    eyebrow: 'LOVE & MEMORIES',
    description: '把日常的陪伴，写成只属于你们的电影。',
    occasions: ['anniversary', 'memories', 'distance'],
    briefLabel: '共同回忆与纪念信息',
    briefHint: '纪念日、相处片段、共同旅行和希望对方听见的话；不必包含婚礼。',
    titleHint: '例如：和你一起的第十年',
    prompt:
      '从日常早餐、共同散步到窗前相伴，用自然动作串起彼此的陪伴。保持日常造型，不添加求婚或婚礼；结尾是一句温柔寄语。',
    duration: 60,
    style: 'romantic',
    ratio: '16:9',
  },
  {
    id: 'commerce',
    name: '电商营销',
    eyebrow: 'PRODUCT & BRAND',
    description: '让商品成为主角，清晰展示价值与使用体验。',
    occasions: ['product', 'tutorial', 'brand'],
    briefLabel: '商品资料、受众与真实卖点',
    briefHint:
      '商品名称、外观/包装、材质、已证实参数、目标人群、使用步骤与行动指引。勿填写未经核实的功效。',
    titleHint: '例如：一杯咖啡的好时光',
    prompt:
      '按 H3 带货分镜写 15 秒 9:16 完整 prompt：素材绑定→整体设定（音色写清语速偏快、节奏紧凑）→镜头N[00:00-00:15] 时间轴。口播与商品特写交替，台词按约 4.5–5.5 字/秒自检，句间停顿短，可跨镜接上句。换品不残留旧话术；无新增字幕，卖点与 CTA 仅来自资料。',
    duration: 15,
    style: 'editorial',
    ratio: '9:16',
  },
];
export function sceneChoice(id?: string) {
  return sceneChoices.find((s) => s.id === id) || sceneChoices[0];
}
export function sceneOccasions(id?: string) {
  return Object.fromEntries(
    sceneChoice(id).occasions.map((k) => [k, occasions[k]]),
  );
}
export function sceneExamples(id?: string) {
  if (id === 'commerce')
    return [
      { name: '商品展示', text: sceneChoice(id).prompt },
      {
        name: '使用演示',
        text: '15秒使用演示：快节奏口播。镜头1 短句吐槽，镜头2–3 手部特写配合紧凑画外说明，镜头4 快收产品陈列。语速偏快，跨镜接上句；每镜台词≤秒数×5.5字；道具拿放有交代。',
      },
      {
        name: '真人种草',
        text: '15秒真人种草：语速偏快、像直播间赶节奏。人物卡+商品图绑定；中景口播与特写硬切，同一声音跨镜。写清入画与标签朝向；台词短句口语化，只基于已提供卖点。',
      },
    ];
  return id && id !== 'wedding'
    ? [{ name: `${sceneChoice(id).name}灵感`, text: sceneChoice(id).prompt }]
    : promptExamples;
}
export function changeScene<T extends ProjectDraft>(draft: T, id: string): T {
  const s = sceneChoice(id);
  return {
    ...draft,
    scene: s.id,
    occasion: s.occasions[0],
    duration: s.duration,
    style: s.style,
    ratio: s.ratio,
    wardrobeMode: s.id === 'commerce' ? 'fixed' : 'auto',
  };
}

export function projectDraft(project: Project): ProjectDraft {
  return {
    creationMode: project.creationMode || 'agent',
    templateId: project.templateId || '',
    scene: project.scene || 'wedding',
    title: project.autoTitle ? '' : project.title,
    brief: project.brief,
    occasion: project.occasion || sceneChoice(project.scene).occasions[0],
    customPrompt: project.customPrompt || '',
    wardrobeMode:
      project.scene === 'commerce' ? 'fixed' : project.wardrobeMode || 'auto',
    wardrobePrompt: project.wardrobePrompt || '',
    endingText: project.endingText || '',
    duration: project.scene === 'commerce' ? 15 : project.duration,
    style:
      project.style === 'garden'
        ? 'romantic'
        : project.style === 'seaside'
          ? 'travel'
          : project.style,
    ratio: project.ratio,
  };
}
