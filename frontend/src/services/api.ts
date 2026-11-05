import axios, { AxiosError, InternalAxiosRequestConfig } from 'axios';
import { authExpiredBus, globalErrorBus } from './eventBus';
import { getEnv } from '../utils/env';
import { ProblemDetails } from '../types';
import { getStoredToken } from './tokenStorage';

declare module 'axios' {
    export interface AxiosRequestConfig {
        skipGlobalErrorHandler?: boolean;
        skipAuth?: boolean;
    }
}

const api = axios.create({
    baseURL: getEnv('VITE_API_URL') || '/api',
    headers: {
        'Content-Type': 'application/json',
        Accept: 'application/json',
    },
    timeout: 30_000,
    withCredentials: false,
    paramsSerializer: {
        indexes: null,
    },
});

const DEFAULT_TIMEOUT = 30_000;
const UPLOAD_TIMEOUT = 10 * 60_000;
const UNSAFE_PATH = /[?#\\]|(?:^|\/)(?:\.|%2e){1,2}(?:\/|$)/i;

api.interceptors.request.use(
    (config) => {
        if (config.url && UNSAFE_PATH.test(config.url)) {
            return Promise.reject(new AxiosError('Refused to call an unexpected API path', AxiosError.ERR_BAD_REQUEST, config));
        }
        const token = config.skipAuth ? null : getStoredToken();
        if (token) {
            config.headers.Authorization = `Bearer ${token}`;
        }
        if (typeof FormData !== 'undefined' && config.data instanceof FormData) {
            config.headers.delete('Content-Type');
            if (!config.timeout || config.timeout === DEFAULT_TIMEOUT) config.timeout = UPLOAD_TIMEOUT;
        }
        return config;
    },
    (error) => Promise.reject(error)
);

export function formatApiError(error: AxiosError<ProblemDetails>): { message: string; type: 'error' | 'warning' } {
    let message = 'An unexpected error occurred';
    let type: 'error' | 'warning' = 'error';

    if (error.response) {
        const { data, status } = error.response;

        if (data?.errors) {
            const validationErrors = Object.values(data.errors).flat();
            if (validationErrors.length > 0) {
                message = String(validationErrors[0]);
                type = 'warning';
            }
        } else if (data?.detail) {
            message = data.detail;
        } else if (data?.title) {
            message = data.title;
        } else if (data?.message) {
            message = data.message;
        } else if (status === 429) {
            message = 'You are sending requests too quickly. Please wait a moment and try again.';
            type = 'warning';
        } else if (status === 403) {
            message = 'You do not have permission to perform this action.';
        } else if (error.response.statusText) {
            message = `${status}: ${error.response.statusText}`;
        }
    } else if (error.code === 'ECONNABORTED') {
        message = 'The request timed out. Please try again.';
    } else if (error.request) {
        message = 'Server unreachable. Please check your internet connection.';
    }

    if (message.length > 300) message = message.slice(0, 300) + '…';
    return { message, type };
}

export function httpStatus(error: unknown): number | undefined {
    return axios.isAxiosError(error) ? error.response?.status : undefined;
}

export function showApiError(error: unknown) {
    const { message, type } = axios.isAxiosError<ProblemDetails>(error)
        ? formatApiError(error)
        : { message: 'An unexpected error occurred', type: 'error' as const };
    globalErrorBus.emit({ message, type });
}

api.interceptors.response.use(
    (response) => response,
    (error: AxiosError<ProblemDetails>) => {
        if (axios.isCancel(error)) {
            return Promise.reject(error);
        }
        const config = error.config as (InternalAxiosRequestConfig & { skipGlobalErrorHandler?: boolean }) | undefined;

        if (error.response?.status === 401) {
            const current = getStoredToken();
            if (current && config?.headers?.Authorization === `Bearer ${current}`) {
                authExpiredBus.emit();
            }
            error.message = 'Your session has expired. Please log in again.';
            return Promise.reject(error);
        }

        const { message } = formatApiError(error);
        error.message = message;

        if (!config?.skipGlobalErrorHandler) {
            showApiError(error);
        }

        return Promise.reject(error);
    }
);

export default api;
