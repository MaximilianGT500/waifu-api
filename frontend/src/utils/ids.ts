const ID_PATTERN = /^[1-9]\d{0,18}$/;

export const isId = (value: string | undefined): value is string => value !== undefined && ID_PATTERN.test(value);
