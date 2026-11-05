type ErrorEvent = {
  message: string;
  type: 'error' | 'warning';
};

type Listener<T> = (event: T) => void;

class EventBus<T> {
  private listeners: Listener<T>[] = [];

  subscribe(listener: Listener<T>) {
    this.listeners.push(listener);
    return () => {
      this.listeners = this.listeners.filter((l) => l !== listener);
    };
  }

  emit(event: T) {
    this.listeners.forEach((listener) => listener(event));
  }
}

export const globalErrorBus = new EventBus<ErrorEvent>();

export const authExpiredBus = new EventBus<void>();
