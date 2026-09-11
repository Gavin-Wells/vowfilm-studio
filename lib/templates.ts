import catalog from '@/server/internal/templates/catalog.json';
import { defaultDraft, type ProjectDraft } from '@/lib/creative';

export const filmTemplates = catalog;
export type FilmTemplate = (typeof filmTemplates)[number];

export function templateDraft(template: FilmTemplate): ProjectDraft {
  return {
    ...defaultDraft(),
    creationMode: 'template',
    templateId: template.id,
    scene: template.scene,
    duration: template.duration,
    ratio: template.ratio,
    style: template.style,
    occasion: template.occasion,
    wardrobeMode: template.wardrobeMode,
  };
}
