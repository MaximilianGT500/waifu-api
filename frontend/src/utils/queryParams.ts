type ParamValue = string | number | boolean | undefined | null;

interface BuildQueryParamsOptions {
  params?: Record<string, ParamValue>;
  arrays?: Record<string, (string | number)[] | undefined>;
}

export function buildQueryParams(options: BuildQueryParamsOptions): URLSearchParams {
  const searchParams = new URLSearchParams();

  if (options.params) {
    for (const [key, value] of Object.entries(options.params)) {
      if (value !== undefined && value !== null && value !== '') {
        searchParams.append(key, String(value));
      }
    }
  }

  if (options.arrays) {
    for (const [key, values] of Object.entries(options.arrays)) {
      if (values && values.length > 0) {
        for (const value of values) {
          searchParams.append(key, String(value));
        }
      }
    }
  }

  return searchParams;
}
