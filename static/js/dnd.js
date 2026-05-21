// PointerEvent-based Drag and Drop for Kanban Cards
class KanbanDragAndDrop {
  constructor(options = {}) {
    this.onCardDropped = options.onCardDropped || (() => {});
    this.activeDrag = null;
    
    // Bind event handlers
    this.handlePointerDown = this.handlePointerDown.bind(this);
    this.handlePointerMove = this.handlePointerMove.bind(this);
    this.handlePointerUp = this.handlePointerUp.bind(this);
  }

  init() {
    // Listen for pointer events on lists container
    const container = document.getElementById('lists-container');
    if (container) {
      container.addEventListener('pointerdown', this.handlePointerDown);
    }
  }

  destroy() {
    const container = document.getElementById('lists-container');
    if (container) {
      container.removeEventListener('pointerdown', this.handlePointerDown);
    }
    this.cleanup();
  }

  handlePointerDown(e) {
    // Only left click / main pointer button
    if (e.button !== 0) return;

    // Find if clicked on card item
    const cardEl = e.target.closest('.card-item');
    if (!cardEl) return;

    // Ignore if clicked on inputs, buttons, or interactive content inside card
    if (e.target.closest('button') || e.target.closest('a') || e.target.closest('.avatar-circle')) {
      return;
    }

    e.preventDefault();

    const rect = cardEl.getBoundingClientRect();
    const listEl = cardEl.closest('.list-column');
    const cardsContainer = cardEl.closest('.cards-container');
    
    // Create placeholder element to mark drop location
    const placeholder = document.createElement('div');
    placeholder.className = 'card-placeholder';
    placeholder.style.height = `${rect.height}px`;

    // Create ghost element that floats under pointer
    const ghost = cardEl.cloneNode(true);
    ghost.classList.add('card-ghost');
    ghost.style.width = `${rect.width}px`;
    ghost.style.height = `${rect.height}px`;
    ghost.style.left = `${rect.left}px`;
    ghost.style.top = `${rect.top}px`;
    document.body.appendChild(ghost);

    // Track state
    this.activeDrag = {
      cardEl,
      listEl,
      cardsContainer,
      cardId: cardEl.dataset.id,
      startListId: listEl.dataset.id,
      placeholder,
      ghost,
      offsetX: e.clientX - rect.left,
      offsetY: e.clientY - rect.top,
      currentListEl: listEl,
      pointerId: e.pointerId
    };

    // Mark original card as dragging
    cardEl.classList.add('dragging');
    
    // Insert placeholder where card was
    cardEl.parentNode.insertBefore(placeholder, cardEl);

    // Hide original card
    cardEl.style.display = 'none';

    // Capture pointer events
    document.body.addEventListener('pointermove', this.handlePointerMove);
    document.body.addEventListener('pointerup', this.handlePointerUp);
    document.body.addEventListener('pointercancel', this.handlePointerUp);
    document.body.setPointerCapture(e.pointerId);
  }

  handlePointerMove(e) {
    if (!this.activeDrag) return;
    
    const drag = this.activeDrag;
    e.preventDefault();

    // Move ghost card
    const x = e.clientX - drag.offsetX;
    const y = e.clientY - drag.offsetY;
    drag.ghost.style.left = `${x}px`;
    drag.ghost.style.top = `${y}px`;

    // Find element under pointer (temporarily hide ghost to see underneath)
    drag.ghost.style.display = 'none';
    const hoverEl = document.elementFromPoint(e.clientX, e.clientY);
    drag.ghost.style.display = 'block';

    if (!hoverEl) return;

    // Check if hovering over another card
    const hoverCard = hoverEl.closest('.card-item');
    // Check if hovering over a list column or list cards container
    const hoverList = hoverEl.closest('.list-column');

    if (hoverList) {
      const container = hoverList.querySelector('.cards-container');
      drag.currentListEl = hoverList;

      if (hoverCard && hoverCard !== drag.cardEl && hoverCard.parentNode === container) {
        // We are hovering over a card in a list. Decide if we insert before or after.
        const rect = hoverCard.getBoundingClientRect();
        const middleY = rect.top + rect.height / 2;
        
        if (e.clientY < middleY) {
          container.insertBefore(drag.placeholder, hoverCard);
        } else {
          container.insertBefore(drag.placeholder, hoverCard.nextSibling);
        }
      } else if (container && !container.contains(drag.placeholder)) {
        // Empty list or hovering on list outer margins
        container.appendChild(drag.placeholder);
      }
    }
  }

  handlePointerUp(e) {
    if (!this.activeDrag) return;

    const drag = this.activeDrag;
    e.preventDefault();

    // Release pointer capture
    try {
      document.body.releasePointerCapture(drag.pointerId);
    } catch (err) {}

    // Remove event listeners
    document.body.removeEventListener('pointermove', this.handlePointerMove);
    document.body.removeEventListener('pointerup', this.handlePointerUp);
    document.body.removeEventListener('pointercancel', this.handlePointerUp);

    // Final calculations
    const parent = drag.placeholder.parentNode;
    const endListId = drag.currentListEl.dataset.id;
    
    // Insert original card back at placeholder position
    parent.insertBefore(drag.cardEl, drag.placeholder);
    
    // Calculate new position index within the list
    const cards = Array.from(parent.querySelectorAll('.card-item:not(.card-ghost)'));
    const newPosition = cards.indexOf(drag.cardEl);

    // Cleanup drag indicators
    drag.cardEl.classList.remove('dragging');
    drag.cardEl.style.display = 'block';
    
    if (drag.placeholder.parentNode) {
      drag.placeholder.parentNode.removeChild(drag.placeholder);
    }
    if (drag.ghost.parentNode) {
      drag.ghost.parentNode.removeChild(drag.ghost);
    }

    const cardId = drag.cardId;
    const startListId = drag.startListId;

    this.activeDrag = null;

    // Check if card was actually moved to a new list or position
    this.onCardDropped(cardId, startListId, endListId, newPosition);
  }

  cleanup() {
    if (this.activeDrag) {
      const drag = this.activeDrag;
      drag.cardEl.classList.remove('dragging');
      drag.cardEl.style.display = 'block';
      if (drag.placeholder.parentNode) {
        drag.placeholder.parentNode.removeChild(drag.placeholder);
      }
      if (drag.ghost.parentNode) {
        drag.ghost.parentNode.removeChild(drag.ghost);
      }
      this.activeDrag = null;
    }
  }
}

// Bind to window for global access
window.KanbanDragAndDrop = KanbanDragAndDrop;
