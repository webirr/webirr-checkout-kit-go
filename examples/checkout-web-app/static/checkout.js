(function () {
  const merchantReference = window.webirrCheckout.merchantReference;
  const review = document.getElementById("review");
  const waiting = document.getElementById("waiting");
  const paid = document.getElementById("paid");
  const checkoutButton = document.getElementById("checkout");
  const refreshButton = document.getElementById("refresh");
  const statusText = document.getElementById("status-text");
  let checkoutData = null;
  let pollTimer = null;
  let inFlight = false;
  let completed = false;

  async function requestJSON(url, options) {
    const response = await fetch(url, options);
    const data = await response.json();
    if (!response.ok || data.error) {
      throw new Error(data.error || "Request failed");
    }
    return data;
  }

  function show(section) {
    review.classList.toggle("hidden", section !== "review");
    waiting.classList.toggle("hidden", section !== "waiting");
    paid.classList.toggle("hidden", section !== "paid");
  }

  function money(amount, currency) {
    return (currency || "ETB") + " " + amount;
  }

  function renderWaiting(data) {
    document.getElementById("payment-code").textContent = data.paymentCode;
    document.getElementById("waiting-customer").textContent = data.customerName || "";
    document.getElementById("waiting-amount").textContent = money(data.amount, data.currency);
    document.getElementById("waiting-reference").textContent = data.merchantReference;
    document.getElementById("waiting-status").textContent = "pending";
    const list = document.getElementById("instructions");
    list.innerHTML = "";
    (data.supportedBanks || []).forEach((bank) => {
      const item = document.createElement("li");
      item.textContent = bank.name + " -> WeBirr -> Payment Code";
      list.appendChild(item);
    });
    refreshButton.classList.add("hidden");
    show("waiting");
  }

  function renderPaid(status) {
    document.getElementById("paid-customer").textContent = status.customerName || checkoutData.customerName || "";
    document.getElementById("paid-amount").textContent = money(status.amount || checkoutData.amount, status.currency || checkoutData.currency);
    document.getElementById("paid-reference").textContent = status.paymentReference || "";
    document.getElementById("paid-via").textContent = status.paymentIssuer || "";
    show("paid");
  }

  function schedulePoll(delay) {
    if (completed) {
      return;
    }
    clearTimeout(pollTimer);
    pollTimer = setTimeout(() => {
      void poll();
    }, delay);
  }

  async function poll() {
    if (inFlight || completed) {
      return;
    }
    inFlight = true;
    refreshButton.disabled = true;
    try {
      const status = await requestJSON("/webirr/checkout/status?merchantReference=" + encodeURIComponent(merchantReference));
      if (status.status === "Paid") {
        completed = true;
        clearTimeout(pollTimer);
        renderPaid(status);
        return;
      }
      statusText.textContent = "Payment not received yet.";
      refreshButton.classList.add("hidden");
      schedulePoll(Math.max(checkoutData.pollIntervalMs || 3000, 1000));
    } catch (error) {
      statusText.textContent = "Could not refresh payment status. Try again.";
      refreshButton.classList.remove("hidden");
    } finally {
      inFlight = false;
      refreshButton.disabled = false;
    }
  }

  checkoutButton.addEventListener("click", async () => {
    checkoutButton.disabled = true;
    try {
      checkoutData = await requestJSON("/webirr/checkout", {
        method: "POST",
        headers: {"content-type": "application/json"},
        body: JSON.stringify({merchantReference})
      });
      renderWaiting(checkoutData);
      schedulePoll(1200);
    } catch (error) {
      checkoutButton.disabled = false;
      window.alert(error.message);
    }
  });

  refreshButton.addEventListener("click", () => {
    refreshButton.disabled = true;
    void poll();
  });
}());
