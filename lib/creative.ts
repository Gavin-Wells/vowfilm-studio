export const occasions: Record<string, string> = {
  opening: '开场 / 新人入场前',
  story: '仪式 / 爱情回顾',
  warmup: '入席 / 暖场循环',
};

export const occasionHints: Record<string, string> = {
  opening: '抓住宾客注意 → 关系推进 → 正式亮相。片尾留画、降音乐，方便主持人接话。',
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
