export type User = {
  id: string;
  email: string;
  name: string;
  role: string;
  disabled: boolean;
  balance: number;
  held: number;
  createdAt: string;
};
export type Auth = { user: User; permissions: string[] };
export type Rule = {
  action: string;
  unit: string;
  base: number;
  rate: number;
  minimum: number;
};
export type Pricing = {
  version: number;
  rules: Rule[];
  sceneFactors: Record<string, number>;
};
export type Quote = {
  id: string;
  projectId: string;
  action: string;
  shotId: string;
  scene: string;
  rule: Rule;
  version: number;
  quantity: number;
  factor: number;
  amount: number;
  expires: number;
};
export type Entry = {
  id: string;
  kind: string;
  delta: number;
  heldDelta: number;
  balanceAfter: number;
  heldAfter: number;
  reference: string;
  note: string;
  actor: string;
  createdAt: string;
};
export type Charge = {
  id: string;
  projectId: string;
  amount: number;
  state: string;
  createdAt: string;
  quote: Quote;
};
export type Wallet = { user: User; entries: Entry[]; charges: Charge[] };
export const roleNames: Record<string, string> = {
  admin: '平台管理员',
  creator: '创作者',
  viewer: '只读成员',
  finance: '财务人员',
};
export const actionNames: Record<string, string> = {
  plan: '导演与分镜编排',
  review: '导演审阅',
  generate: '一键生成影片',
  shot: '局部镜头重做',
  render: '合成影片',
};
export const unitNames: Record<string, string> = {
  call: '按次',
  task: '按任务',
  second: '按秒',
  shot: '按镜头',
};
export const credits = (n: number) =>
  (n / 1000).toLocaleString('zh-CN', { maximumFractionDigits: 3 });
export const dateTime = (s: string) => new Date(s).toLocaleString('zh-CN');

export function formText(form: FormData, key: string): string {
  const value = form.get(key);
  return typeof value === 'string' ? value : '';
}
