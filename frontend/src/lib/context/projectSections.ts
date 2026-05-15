import type { ProjectSection } from '$lib/api/types';

export const PROJECT_SECTIONS_KEY = 'turboist:projectSections';

export interface ProjectSectionsCtx {
	readonly sections: ProjectSection[];
}
