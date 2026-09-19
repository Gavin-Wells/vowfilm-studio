import catalog from '@/server/internal/templates/catalog.json';
import { defaultDraft, type ProjectDraft } from '@/lib/creative';

export type FilmTemplate = (typeof catalog)[number] & {
  engine?: string;
  previewUrl?: string;
  posterUrl?: string;
};
export const filmTemplates: FilmTemplate[] = catalog;

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
