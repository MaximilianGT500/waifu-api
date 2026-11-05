import { useImages, ImageFilters } from './useImages';
import { ImageDto, NsfwMode } from '../types';

export const useAlternativeSearch = (
    isLoading: boolean,
    images: ImageDto[] | undefined,
    isNsfw: NsfwMode,
    filters: ImageFilters
) => {
    const shouldCheck = !isLoading && images?.length === 0 && isNsfw === NsfwMode.False;

    const { data: alternativeData, isLoading: isCheckingAlternatives } = useImages({
        ...filters,
        isNsfw: NsfwMode.All,
        page: 1,
        pageSize: 1,
        enabled: shouldCheck
    });

    const alternativeCount = shouldCheck && alternativeData?.totalCount && alternativeData.totalCount > 0
        ? alternativeData.totalCount
        : null;

    return {
        alternativeCount,
        isCheckingAlternatives: shouldCheck && isCheckingAlternatives
    };
};
