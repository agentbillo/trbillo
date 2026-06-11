// Real-time WebSocket Client for Trbillo
class TrbilloWSClient {
  constructor() {
    this.socket = null;
    this.boardId = null;
    this.reconnectTimeout = null;
    this.reconnectDelay = 1000; // start with 1 second delay
    this.maxReconnectDelay = 30000;
  }

  connect(boardId) {
    // Clean up existing connection
    this.disconnect();
    
    this.boardId = boardId;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const base = window.BASE_PATH || '';
    const wsUrl = `${protocol}//${window.location.host}${base}/api/ws?board_id=${boardId}`;
    
    console.log(`Connecting to WebSocket: ${wsUrl}`);
    this.socket = new WebSocket(wsUrl);

    this.socket.onopen = () => {
      console.log('WebSocket connection established.');
      this.reconnectDelay = 1000; // Reset delay on successful connection
    };

    this.socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        // Dispatch custom event to let the main application react
        const wsEvent = new CustomEvent('trbillo-ws-message', { detail: msg });
        document.dispatchEvent(wsEvent);
      } catch (err) {
        console.error('Error parsing WebSocket message:', err);
      }
    };

    this.socket.onclose = (event) => {
      console.log(`WebSocket connection closed: code=${event.code}, reason=${event.reason}`);
      this.socket = null;
      
      // Do not reconnect if disconnected intentionally or board changed
      if (this.boardId === boardId && event.code !== 1000) {
        this.scheduleReconnect();
      }
    };

    this.socket.onerror = (error) => {
      console.error('WebSocket error encountered:', error);
    };
  }

  disconnect() {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }
    
    if (this.socket) {
      // 1000 indicates a normal closure
      this.socket.close(1000, 'Board changed or user logged out');
      this.socket = null;
    }
    this.boardId = null;
  }

  scheduleReconnect() {
    if (this.reconnectTimeout) return;

    console.log(`Scheduling reconnect in ${this.reconnectDelay}ms...`);
    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      if (this.boardId) {
        this.connect(this.boardId);
        // Exponential backoff
        this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
      }
    }, this.reconnectDelay);
  }
}

// User-level WebSocket Client (for events regardless of which board is viewed)
class TrbilloUserWSClient {
  constructor() {
    this.socket = null;
    this.reconnectTimeout = null;
    this.reconnectDelay = 1000;
    this.maxReconnectDelay = 30000;
    this.isConnected = false;
  }

  connect() {
    // Always tear down any existing socket first. It may have been
    // authenticated as a previous user (e.g. after a session expiry / 401 and
    // re-login as someone else); reusing it would deliver that user's events
    // to the current UI — including a "removed_from_board" meant for them.
    this.disconnect();

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const base = window.BASE_PATH || '';
    const wsUrl = `${protocol}//${window.location.host}${base}/api/ws/user`;

    console.log(`Connecting to User WebSocket: ${wsUrl}`);
    this.socket = new WebSocket(wsUrl);

    this.socket.onopen = () => {
      console.log('User WebSocket connection established.');
      this.isConnected = true;
      this.reconnectDelay = 1000;
    };

    this.socket.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data);
        const wsEvent = new CustomEvent('trbillo-ws-message', { detail: msg });
        document.dispatchEvent(wsEvent);
      } catch (err) {
        console.error('Error parsing User WebSocket message:', err);
      }
    };

    this.socket.onclose = (event) => {
      console.log(`User WebSocket closed: code=${event.code}`);
      this.socket = null;
      this.isConnected = false;

      // Reconnect unless intentionally disconnected
      if (event.code !== 1000) {
        this.scheduleReconnect();
      }
    };

    this.socket.onerror = (error) => {
      console.error('User WebSocket error:', error);
    };
  }

  disconnect() {
    if (this.reconnectTimeout) {
      clearTimeout(this.reconnectTimeout);
      this.reconnectTimeout = null;
    }

    if (this.socket) {
      this.socket.close(1000, 'User logged out');
      this.socket = null;
    }
    this.isConnected = false;
  }

  scheduleReconnect() {
    if (this.reconnectTimeout) return;

    console.log(`Scheduling User WS reconnect in ${this.reconnectDelay}ms...`);
    this.reconnectTimeout = setTimeout(() => {
      this.reconnectTimeout = null;
      this.connect();
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, this.maxReconnectDelay);
    }, this.reconnectDelay);
  }
}

// Instantiate global WS clients
window.wsClient = new TrbilloWSClient();
window.userWsClient = new TrbilloUserWSClient();
