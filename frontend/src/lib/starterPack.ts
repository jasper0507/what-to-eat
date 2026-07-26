// 经典起步包（brief §三）：十余道国民家常菜，一键入池默认中档，
// 十秒可体验首次揭示。id 逐一对照过当前 HowToCook 缓存（369 谱）。
export interface StarterDish {
  id: string;
  name: string;
}

export const STARTER_PACK: readonly StarterDish[] = [
  { id: "vegetable_dish/西红柿炒鸡蛋.md", name: "西红柿炒鸡蛋" },
  { id: "meat_dish/鱼香肉丝.md", name: "鱼香肉丝" },
  { id: "meat_dish/宫保鸡丁/宫保鸡丁.md", name: "宫保鸡丁" },
  { id: "meat_dish/红烧肉/南派红烧肉.md", name: "南派红烧肉" },
  { id: "meat_dish/麻婆豆腐/麻婆豆腐.md", name: "麻婆豆腐" },
  { id: "meat_dish/可乐鸡翅.md", name: "可乐鸡翅" },
  { id: "meat_dish/回锅肉/回锅肉.md", name: "回锅肉" },
  { id: "meat_dish/糖醋里脊.md", name: "糖醋里脊" },
  { id: "vegetable_dish/酸辣土豆丝.md", name: "酸辣土豆丝" },
  { id: "vegetable_dish/蒜蓉西兰花.md", name: "蒜蓉西兰花" },
  { id: "vegetable_dish/红烧茄子.md", name: "红烧茄子" },
  { id: "vegetable_dish/手撕包菜/手撕包菜.md", name: "手撕包菜" },
  { id: "soup/西红柿鸡蛋汤.md", name: "西红柿鸡蛋汤" },
  { id: "staple/蛋炒饭.md", name: "蛋炒饭" },
];
